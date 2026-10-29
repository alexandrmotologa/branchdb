package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
)

// WireProxy acts as a transparent PostgreSQL wire protocol proxy.
type WireProxy struct {
	cfg      *config.Config
	router   *Router
	metrics  *Metrics
	queries  *QueryRingBuffer
	listener net.Listener
	stopCh   chan struct{}
	wg       sync.WaitGroup
	closed   atomic.Bool
}

// NewWireProxy creates a new PostgreSQL wire proxy instance.
func NewWireProxy(cfg *config.Config, router *Router, metrics *Metrics) *WireProxy {
	return &WireProxy{
		cfg:     cfg,
		router:  router,
		metrics: metrics,
		queries: NewQueryRingBuffer(1000),
		stopCh:  make(chan struct{}),
	}
}

// QueryLog returns the in-memory circular query log.
func (p *WireProxy) QueryLog() *QueryRingBuffer {
	return p.queries
}

// Start binds to the configured port and starts accepting incoming client connections.
func (p *WireProxy) Start(ctx context.Context) error {
	addr := p.cfg.ProxyAddr()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed listening on %s: %w", addr, err)
	}

	p.listener = l
	log.Printf("[wire-proxy] Listening on PostgreSQL wire port %s (routing to %s)", addr, p.cfg.UpstreamAddr())

	go func() {
		<-ctx.Done()
		p.Stop()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if p.closed.Load() {
				return nil
			}
			log.Printf("[wire-proxy] Accept error: %v", err)
			continue
		}

		p.wg.Add(1)
		go func(c net.Conn) {
			defer p.wg.Done()
			p.handleConnection(ctx, c)
		}(conn)
	}
}

// Stop cleanly closes the listener and waits for active connections to finish.
func (p *WireProxy) Stop() {
	if p.closed.CompareAndSwap(false, true) {
		close(p.stopCh)
		if p.listener != nil {
			_ = p.listener.Close()
		}
	}
}

func (p *WireProxy) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))
	pkt, err := ReadStartupPacket(clientConn)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("[wire-proxy] Failed reading client startup packet: %v", err)
		}
		return
	}

	// Handle SSLRequest negotiation: tell client SSL is unsupported on local proxy
	if pkt.IsSSLRequest {
		// PostgreSQL protocol specifies single byte 'N' for SSL not supported
		if _, err := clientConn.Write([]byte{'N'}); err != nil {
			return
		}
		// Read the actual StartupMessage following SSL denial
		_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))
		pkt, err = ReadStartupPacket(clientConn)
		if err != nil {
			log.Printf("[wire-proxy] Failed reading startup message after SSL denial: %v", err)
			return
		}
	}

	// Handle CancelRequest: forward directly to upstream without routing changes
	if pkt.IsCancelRequest {
		upstreamConn, err := net.DialTimeout("tcp", p.cfg.UpstreamAddr(), 3*time.Second)
		if err != nil {
			return
		}
		defer upstreamConn.Close()
		_, _ = upstreamConn.Write(pkt.RawPayload)
		return
	}

	// Standard StartupMessage: resolve target database for current active Git branch
	requestedDb := pkt.Parameters["database"]
	activeBranch, targetDb, err := p.router.ResolveTargetDatabase(ctx, requestedDb)
	if err != nil {
		log.Printf("[wire-proxy] Route resolution failed for branch %s: %v", activeBranch, err)
		return
	}

	// Rewrite database parameter
	pkt.Parameters["database"] = targetDb
	rewrittenPayload := EncodeStartupMessage(pkt.Parameters)

	// Dial upstream PostgreSQL server
	upstreamConn, err := net.DialTimeout("tcp", p.cfg.UpstreamAddr(), 5*time.Second)
	if err != nil {
		log.Printf("[wire-proxy] Failed connecting to upstream PostgreSQL at %s: %v", p.cfg.UpstreamAddr(), err)
		return
	}
	defer upstreamConn.Close()

	// Clear deadlines for persistent session
	_ = clientConn.SetDeadline(time.Time{})
	_ = upstreamConn.SetDeadline(time.Time{})

	// Send rewritten StartupMessage to upstream
	if _, err := upstreamConn.Write(rewrittenPayload); err != nil {
		log.Printf("[wire-proxy] Failed sending startup packet to upstream: %v", err)
		return
	}

	// Register connection metrics
	p.metrics.ConnOpened(targetDb)
	defer p.metrics.ConnClosed(targetDb)

	log.Printf("[wire-proxy] Routed connection: client=%s branch=%s target_db=%s",
		clientConn.RemoteAddr(), activeBranch, targetDb)

	// Bridge connections bidirectionally
	var bridgeWg sync.WaitGroup
	bridgeWg.Add(2)

	// Client -> Upstream (with query inspection)
	go func() {
		defer bridgeWg.Done()
		defer upstreamConn.Close()

		buf := make([]byte, 32*1024)
		for {
			n, err := clientConn.Read(buf)
			if n > 0 {
				p.inspectClientMessages(buf[:n], activeBranch, targetDb, clientConn.RemoteAddr().String())
				p.metrics.AddBytes(int64(n), 0)

				if _, wErr := upstreamConn.Write(buf[:n]); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Upstream -> Client
	go func() {
		defer bridgeWg.Done()
		defer clientConn.Close()

		buf := make([]byte, 32*1024)
		for {
			n, err := upstreamConn.Read(buf)
			if n > 0 {
				p.metrics.AddBytes(0, int64(n))

				if _, wErr := clientConn.Write(buf[:n]); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	bridgeWg.Wait()
}

// inspectClientMessages detects PostgreSQL Query ('Q') and Parse ('P') packets, recording them into the query buffer.
func (p *WireProxy) inspectClientMessages(payload []byte, branch, database, clientAddr string) {
	if len(payload) == 0 {
		return
	}
	msgType := payload[0]
	// 'Q' = Simple Query, 'P' = Parse (Extended Query Protocol)
	if msgType == 'Q' || msgType == 'P' {
		p.metrics.QueryExecuted()
		p.router.TouchDatabase(database)
		if sql, ok := ExtractSQLFromPacket(payload); ok && sql != "" {
			p.queries.Record(branch, database, clientAddr, sql, 0.35)
		}
	}
}

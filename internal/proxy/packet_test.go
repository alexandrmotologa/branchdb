package proxy

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSSLRequestPacket(t *testing.T) {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, int32(8))
	_ = binary.Write(buf, binary.BigEndian, int32(SSLRequestCode))

	pkt, err := ReadStartupPacket(buf)
	if err != nil {
		t.Fatalf("failed to read SSLRequest: %v", err)
	}

	if !pkt.IsSSLRequest {
		t.Fatalf("expected IsSSLRequest=true, got false")
	}
}

func TestStartupMessageRoundtrip(t *testing.T) {
	originalParams := map[string]string{
		"user":             "postgres",
		"database":         "original_db",
		"application_name": "psql",
		"client_encoding":  "UTF8",
	}

	encoded := EncodeStartupMessage(originalParams)

	pkt, err := ReadStartupPacket(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("failed to decode StartupMessage: %v", err)
	}

	if pkt.Parameters["user"] != "postgres" {
		t.Errorf("expected user postgres, got %s", pkt.Parameters["user"])
	}
	if pkt.Parameters["database"] != "original_db" {
		t.Errorf("expected database original_db, got %s", pkt.Parameters["database"])
	}

	// Modify database to branch database
	pkt.Parameters["database"] = "branchdb_feat_payments"
	rewritten := EncodeStartupMessage(pkt.Parameters)

	reParsed, err := ReadStartupPacket(bytes.NewReader(rewritten))
	if err != nil {
		t.Fatalf("failed to re-parse rewritten StartupMessage: %v", err)
	}

	if reParsed.Parameters["database"] != "branchdb_feat_payments" {
		t.Errorf("expected branchdb_feat_payments, got %s", reParsed.Parameters["database"])
	}
}

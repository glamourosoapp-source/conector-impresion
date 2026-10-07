package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testToken = "tok123"

type fakePrinters struct {
	printed  []byte
	printer  string
	failWith error
}

func newTestServer(t *testing.T) (*server, *fakePrinters) {
	t.Helper()
	fake := &fakePrinters{}
	srv := &server{
		config: agentConfig{
			port:           0,
			token:          testToken,
			allowedOrigins: []string{"https://glamouroso.app"},
			dataDir:        t.TempDir(),
		},
		printers: printerBackend{
			list: func() ([]printerInfo, error) {
				return []printerInfo{{Name: "EPSON TM-T20", IsDefault: true, Status: "Lista"}}, nil
			},
			print: func(name string, data []byte) error {
				if fake.failWith != nil {
					return fake.failWith
				}
				fake.printer = name
				fake.printed = data
				return nil
			},
		},
		hostname: "CAJA-1",
		logf:     func(string, map[string]interface{}) {},
	}
	return srv, fake
}

func do(srv *server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

var auth = map[string]string{"Authorization": "Bearer " + testToken, "Origin": "https://glamouroso.app"}

func TestHealthSinToken(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["name"] != "glamouroso-print-agent" || body["version"] != version || body["hostname"] != "CAJA-1" {
		t.Fatalf("health inesperado: %v", body)
	}
}

func TestCORSSoloOrigenPermitido(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, http.MethodOptions, "/print", "", map[string]string{
		"Origin":                                 "https://glamouroso.app",
		"Access-Control-Request-Private-Network": "true",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://glamouroso.app" {
		t.Fatal("no permitió el origen de la caja")
	}
	if rec.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatal("no concedió Private Network Access")
	}

	rec = do(srv, http.MethodOptions, "/print", "", map[string]string{
		"Origin":                                 "https://malicioso.example",
		"Access-Control-Request-Private-Network": "true",
	})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Private-Network") != "" {
		t.Fatal("permitió un origen fuera de la lista blanca")
	}
}

func TestTokenObligatorio(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/printers", "/pos/backup"} {
		rec := do(srv, http.MethodGet, path, "", map[string]string{"Authorization": "Bearer otro"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s sin token válido respondió %d", path, rec.Code)
		}
	}
}

func TestListaImpresoras(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, http.MethodGet, "/printers", "", auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Printers []printerInfo `json:"printers"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Printers) != 1 || body.Printers[0].Name != "EPSON TM-T20" || !body.Printers[0].IsDefault {
		t.Fatalf("impresoras inesperadas: %s", rec.Body.String())
	}
}

func TestListaVaciaEsArreglo(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.printers.list = func() ([]printerInfo, error) { return nil, nil }
	rec := do(srv, http.MethodGet, "/printers", "", auth)
	if !strings.Contains(rec.Body.String(), `"printers":[]`) {
		t.Fatalf("la caja espera un arreglo, llegó %s", rec.Body.String())
	}
}

func TestImprimeBytesTalCual(t *testing.T) {
	srv, fake := newTestServer(t)
	// ESC @, texto, corte parcial: tiene que llegar idéntico a la impresora.
	ticket := []byte{0x1b, 0x40, 'H', 'o', 'l', 'a', 0x1d, 0x56, 0x42, 0x00}
	payload, _ := json.Marshal(map[string]string{
		"printerName": "EPSON TM-T20",
		"dataBase64":  b64(ticket),
	})
	rec := do(srv, http.MethodPost, "/print", string(payload), auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if fake.printer != "EPSON TM-T20" || !bytes.Equal(fake.printed, ticket) {
		t.Fatalf("llegó %q a %q", fake.printed, fake.printer)
	}
}

func TestImprimeValidaYReportaErrores(t *testing.T) {
	srv, fake := newTestServer(t)
	rec := do(srv, http.MethodPost, "/print", `{"printerName":"X"}`, auth)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin datos respondió %d", rec.Code)
	}
	fake.failWith = errors.New("No se pudo abrir la impresora")
	rec = do(srv, http.MethodPost, "/print", `{"printerName":"X","dataBase64":"AA=="}`, auth)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "No se pudo abrir") {
		t.Fatalf("error de impresora no llegó a la caja: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRespaldoIdaYVuelta(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := do(srv, http.MethodGet, "/pos/backup", "", auth)
	if !strings.Contains(rec.Body.String(), `"outbox":[]`) {
		t.Fatalf("sin respaldo debe devolver cola vacía: %s", rec.Body.String())
	}

	body := `{"outbox":[{"id":"e1","type":"sale.created"},{"id":"e2"}],"counter":{"next":7},"meta":{"branchId":"b1"},"savedAt":"2026-10-06T00:00:00.000Z"}`
	rec = do(srv, http.MethodPut, "/pos/backup", body, auth)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pending":2`) {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(srv.config.dataDir, "pos-backup.json.tmp")); !os.IsNotExist(err) {
		t.Fatal("quedó el temporal: la escritura no fue atómica")
	}

	rec = do(srv, http.MethodGet, "/pos/backup", "", auth)
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got["outbox"].([]interface{})) != 2 || got["savedAt"] != "2026-10-06T00:00:00.000Z" {
		t.Fatalf("respaldo distinto al guardado: %s", rec.Body.String())
	}
}

func TestRespaldoInvalidoOGrande(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, body := range []string{`{}`, `{"outbox":{}}`, `no es json`, `null`} {
		if rec := do(srv, http.MethodPut, "/pos/backup", body, auth); rec.Code != http.StatusBadRequest {
			t.Fatalf("%q respondió %d", body, rec.Code)
		}
	}
	big := `{"outbox":["` + strings.Repeat("x", maxBackupBytes) + `"]}`
	if rec := do(srv, http.MethodPut, "/pos/backup", big, auth); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("respaldo enorme respondió %d", rec.Code)
	}
}

func TestRespaldoCorruptoSeIgnora(t *testing.T) {
	srv, _ := newTestServer(t)
	_ = os.WriteFile(backupPath(srv.config.dataDir), []byte(`{"outbox":[`), 0o644)
	rec := do(srv, http.MethodGet, "/pos/backup", "", auth)
	if !strings.Contains(rec.Body.String(), `"outbox":[]`) {
		t.Fatalf("respaldo corrupto debe ignorarse: %s", rec.Body.String())
	}
}

func TestTokenSeConservaEntreArranques(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.txt")
	first, err := loadOrCreateToken(path)
	if err != nil || len(first) != 32 {
		t.Fatalf("token %q err %v", first, err)
	}
	second, _ := loadOrCreateToken(path)
	if first != second {
		t.Fatal("el token cambió: las cajas emparejadas se desconectarían")
	}
	// Token heredado de la versión de Bun, con salto de línea.
	_ = os.WriteFile(path, []byte("abc123\n"), 0o644)
	if got, _ := loadOrCreateToken(path); got != "abc123" {
		t.Fatalf("no respetó el token existente: %q", got)
	}
}

func b64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

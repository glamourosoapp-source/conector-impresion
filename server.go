package main

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
)

// printerInfo es lo que la caja muestra en Configuración → Impresora.
// Los nombres de los campos son el contrato con el Front: no cambiarlos.
type printerInfo struct {
	Name      string `json:"name"`
	Driver    string `json:"driver,omitempty"`
	PortName  string `json:"portName,omitempty"`
	IsDefault bool   `json:"isDefault"`
	Status    string `json:"status,omitempty"`
}

// printerBackend separa el servidor de Windows: en las pruebas se cambia por
// uno falso, en Windows es winspool.drv y en Mac/Linux lpstat/lp.
type printerBackend struct {
	list  func() ([]printerInfo, error)
	print func(printerName string, data []byte) error
}

type server struct {
	config   agentConfig
	printers printerBackend
	hostname string
	logf     func(message string, extra map[string]interface{})
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.setCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// El health check no pide token: la caja lo usa para saber si hay conector y
	// no revela nada (nombre, versión y equipo).
	if r.URL.Path == "/health" {
		writeJSON(w, http.StatusOK, map[string]string{
			"name":     "glamouroso-print-agent",
			"version":  version,
			"hostname": s.hostname,
		})
		return
	}

	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, `Token inválido. Búscalo en Inicio → "Ver token del Conector de impresión".`)
		return
	}

	switch {
	case r.URL.Path == "/printers" && r.Method == http.MethodGet:
		s.handlePrinters(w)
	case r.URL.Path == "/print" && r.Method == http.MethodPost:
		s.handlePrint(w, r)
	case r.URL.Path == "/pos/backup" && r.Method == http.MethodPut:
		s.handleBackupPut(w, r)
	case r.URL.Path == "/pos/backup" && r.Method == http.MethodGet:
		s.handleBackupGet(w)
	default:
		writeError(w, http.StatusNotFound, "Ruta no encontrada")
	}
}

func (s *server) setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowed := ""
	for _, candidate := range s.config.allowedOrigins {
		if origin != "" && origin == candidate {
			allowed = origin
			break
		}
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", allowed)
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
	h.Set("Access-Control-Max-Age", "600")
	h.Set("Vary", "Origin")
	// Chrome pide permiso explícito para que una página pública (la caja en
	// https) le hable a loopback (Private Network Access). Solo se concede a los
	// orígenes de la lista blanca.
	if allowed != "" && r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		h.Set("Access-Control-Allow-Private-Network", "true")
	}
}

func (s *server) authorized(r *http.Request) bool {
	expected := "Bearer " + s.config.token
	got := r.Header.Get("Authorization")
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func (s *server) handlePrinters(w http.ResponseWriter) {
	printers, err := s.printers.list()
	if err != nil {
		s.logf("listPrinters falló", map[string]interface{}{"error": err.Error()})
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if printers == nil {
		printers = []printerInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"printers": printers})
}

func (s *server) handlePrint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PrinterName string `json:"printerName"`
		DataBase64  string `json:"dataBase64"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBackupBytes)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Cuerpo inválido")
		return
	}
	if body.PrinterName == "" || body.DataBase64 == "" {
		writeError(w, http.StatusBadRequest, "Faltan printerName o dataBase64")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.DataBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "dataBase64 inválido")
		return
	}
	if err := s.printers.print(body.PrinterName, data); err != nil {
		s.logf("print falló", map[string]interface{}{"printer": body.PrinterName, "error": err.Error()})
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logf("Ticket impreso", map[string]interface{}{"printer": body.PrinterName, "bytes": len(data)})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "bytes": len(data)})
}

func (s *server) handleBackupPut(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBackupBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "No se pudo leer el respaldo")
		return
	}
	if len(raw) > maxBackupBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "El respaldo es demasiado grande")
		return
	}
	payload, pending, err := parseBackup(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := writeBackup(s.config.dataDir, payload); err != nil {
		s.logf("respaldo falló", map[string]interface{}{"error": err.Error()})
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "pending": pending})
}

func (s *server) handleBackupGet(w http.ResponseWriter) {
	data := readBackup(s.config.dataDir)
	if data == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"outbox": []interface{}{}, "counter": nil, "meta": nil, "savedAt": nil})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func hostnameOrEmpty() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

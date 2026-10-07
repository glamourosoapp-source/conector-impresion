package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

/*
Respaldo de la cola de la caja.

El navegador guarda las ventas cobradas en IndexedDB, que vive dentro del perfil
de Chrome: basta que alguien "limpie el navegador" para llevarse ventas que
nunca llegaron al servidor. Este conector ya corre en la misma PC para
imprimir, así que es el único lugar donde la caja puede dejar una copia sin
instalar nada más.

El conector **no sube nada al servidor**: no tiene credenciales y no se las
vamos a dar. Es un espejo para restaurar, y nada más.
*/

// Un respaldo absurdamente grande es un error, no una sucursal muy trabajadora.
const maxBackupBytes = 8 * 1024 * 1024

var errInvalidBackup = errors.New("Respaldo inválido")

func backupPath(dataDir string) string {
	return filepath.Join(dataDir, "pos-backup.json")
}

// Valida lo que manda la caja y devuelve cuántos eventos trae la cola. El resto
// del contenido se guarda tal cual: el conector no interpreta las ventas.
func parseBackup(raw []byte) (map[string]json.RawMessage, int, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return nil, 0, errInvalidBackup
	}
	var outbox []json.RawMessage
	rawOutbox, ok := payload["outbox"]
	if !ok || !bytes.HasPrefix(bytes.TrimSpace(rawOutbox), []byte("[")) {
		return nil, 0, errInvalidBackup
	}
	if err := json.Unmarshal(rawOutbox, &outbox); err != nil {
		return nil, 0, errInvalidBackup
	}
	if _, ok := payload["savedAt"]; !ok {
		stamp, _ := json.Marshal(time.Now().UTC().Format(time.RFC3339Nano))
		payload["savedAt"] = stamp
	}
	return payload, len(outbox), nil
}

// Escribe el respaldo de forma atómica: primero un temporal y luego un rename.
// Escribir encima del bueno lo deja a medias si la PC se apaga justo ahí, que es
// exactamente el escenario del que protege.
func writeBackup(dataDir string, payload map[string]json.RawMessage) error {
	target := backupPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	temp := target + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, target)
}

// Devuelve el último respaldo, o nil si no hay. Un respaldo corrupto se ignora:
// la caja arranca vacía, que es mejor que arrancar con datos a medias.
func readBackup(dataDir string) []byte {
	data, err := os.ReadFile(backupPath(dataDir))
	if err != nil || !json.Valid(data) {
		return nil
	}
	return data
}

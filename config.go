package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/*
Configuración del conector. El token se genera la primera vez y se guarda en
disco: es lo que el cajero copia una sola vez en la pantalla de configuración
de la caja para emparejarla con esta PC.

La carpeta (`GlamourosoPrintAgent`) y los nombres de archivo son los mismos que
usaba la versión de Bun: renombrarlos dejaría huérfanos el token y el respaldo
de las cajas ya emparejadas.
*/

const defaultPort = 9377

type agentConfig struct {
	port int
	// Mismo valor que guarda token.txt; viaja en `Authorization: Bearer`.
	token string
	// Orígenes de la caja autorizados a hablarle al conector.
	allowedOrigins []string
	logPath        string
	// Carpeta del conector: token, log y respaldo de la cola de la caja.
	dataDir string
}

func resolveDataDir() string {
	base := firstNonEmpty(os.Getenv("GLAM_AGENT_DATA_DIR"), os.Getenv("ProgramData"), os.Getenv("APPDATA"))
	if base == "" {
		base, _ = os.Getwd()
	}
	return filepath.Join(base, "GlamourosoPrintAgent")
}

func loadConfig() (agentConfig, error) {
	dir := resolveDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return agentConfig{}, err
	}

	token, err := loadOrCreateToken(filepath.Join(dir, "token.txt"))
	if err != nil {
		return agentConfig{}, err
	}

	port := defaultPort
	if raw := os.Getenv("GLAM_AGENT_PORT"); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			port = parsed
		}
	}

	return agentConfig{
		port:           port,
		token:          token,
		allowedOrigins: parseOrigins(firstNonEmpty(os.Getenv("GLAM_ALLOWED_ORIGINS"), "http://localhost:3000,https://glamouroso.app")),
		logPath:        filepath.Join(dir, "agent.log"),
		dataDir:        dir,
	}, nil
}

// El instalador arranca el conector y "Ver token" casi al mismo tiempo: se crea
// en exclusiva para que, si los dos generan uno, gane el primero y la caja no
// quede emparejada con un token que se perdió.
func loadOrCreateToken(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return "", readErr
			}
			return strings.TrimSpace(string(data)), nil
		}
		return "", err
	}
	defer file.Close()
	if _, err := file.WriteString(token); err != nil {
		return "", err
	}
	return token, nil
}

func parseOrigins(raw string) []string {
	var origins []string
	for _, origin := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

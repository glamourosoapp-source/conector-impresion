package main

/*
Conector de impresión del punto de venta de Glamouroso.

Se llama "conector" y no "agente" a propósito: en el CRM "Agente IA" ya es el
agente de WhatsApp, y en una llamada de soporte con una sucursal "no jala el
agente" no puede significar dos cosas.

Corre en la PC de la sucursal y **solo escucha en loopback** (127.0.0.1), así que
nada de la red local puede hablarle. La caja (una página https) sí puede: Chrome
trata loopback como origen potencialmente confiable.

Cada petición exige el token que se generó en la primera ejecución, y el origen
debe estar en la lista blanca. Sin eso, cualquier página abierta en esa PC podría
mandar a imprimir.

Está en Go (desde 1.3.0) y no en Bun porque las PC de las sucursales son viejas:
el ejecutable de Bun exige Windows 10 1809 y en Washington (Windows 10 1607) ni
siquiera arrancaba. Go 1.20 compila para Windows 7 en adelante, de 32 o 64 bits,
y no necesita PowerShell para listar impresoras.

En Windows se compila como aplicación de ventana (`-H windowsgui`): no abre
consola. El token se consulta con `--mostrar-token`, que es lo que abre el acceso
"Ver token del Conector de impresión" del menú Inicio.
*/

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Subirla también en installer/setup.iss (MyAppVersion).
const version = "1.3.0"

func main() {
	config, err := loadConfig()
	if err != nil {
		// Sin carpeta de datos no hay token ni log: no hay dónde avisar.
		os.Exit(1)
	}

	logf := newLogger(config.logPath)

	for _, arg := range os.Args[1:] {
		if arg == "--mostrar-token" {
			if err := showToken(config); err != nil {
				logf("No se pudo mostrar el token", map[string]interface{}{"error": err.Error()})
				os.Exit(1)
			}
			return
		}
	}

	// Loopback a propósito: el conector no se expone a la red de la sucursal.
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", config.port))
	if err != nil {
		// Casi siempre es que ya hay un conector corriendo (el inicio de sesión y
		// el instalador lo arrancaron a la vez, o alguien lo abrió dos veces). Sin
		// consola nadie vería el error: se anota y se sale en silencio, el que ya
		// corre sigue imprimiendo.
		logf(fmt.Sprintf("No se pudo abrir el puerto %d (¿el conector ya está corriendo?)", config.port), map[string]interface{}{"error": err.Error()})
		return
	}

	srv := &server{
		config:   config,
		printers: platformPrinters(),
		hostname: hostnameOrEmpty(),
		logf:     logf,
	}

	logf(fmt.Sprintf("Conector de impresión %s escuchando en http://127.0.0.1:%d", version, config.port), nil)
	logf("Orígenes permitidos: "+strings.Join(config.allowedOrigins, ", "), nil)

	httpServer := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	if err := httpServer.Serve(listener); err != nil {
		logf("El servidor se detuvo", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}

// Sin consola el log es el único rastro: lo que antes salía en la ventana negra
// queda en %ProgramData%\GlamourosoPrintAgent\agent.log.
func newLogger(path string) func(string, map[string]interface{}) {
	return func(message string, extra map[string]interface{}) {
		line := time.Now().UTC().Format(time.RFC3339) + " " + message
		if len(extra) > 0 {
			if encoded, err := json.Marshal(extra); err == nil {
				line += " " + string(encoded)
			}
		}
		fmt.Println(line)
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return // sin permiso de escritura el conector sigue imprimiendo
		}
		defer file.Close()
		_, _ = file.WriteString(line + "\r\n")
	}
}

// Escribe el token con instrucciones en un .txt y lo abre en el Bloc de notas.
// Sin consola no hay otro lugar donde mostrarlo, y el Bloc de notas deja
// seleccionarlo y copiarlo, que es justo lo que hay que hacer con él.
func showToken(config agentConfig) error {
	path := filepath.Join(config.dataDir, "token-para-la-caja.txt")
	content := strings.Join([]string{
		"Glamouroso · Conector de impresión",
		"",
		"Copia este token y pégalo en la caja, en Configuración → Token de emparejamiento:",
		"",
		config.token,
		"",
		"Puedes cerrar esta ventana. El conector sigue funcionando en segundo plano.",
		"",
	}, "\r\n")
	// BOM: el Bloc de notas de Windows 7/10 viejo no detecta UTF-8 sin él y
	// mostraría "Â·" y "â†’" en lugar de los acentos y la flecha.
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, content...), 0o644); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		fmt.Printf("Token para la configuración de la caja: %s\n", config.token)
		return nil
	}
	notepad := filepath.Join(firstNonEmpty(os.Getenv("SystemRoot"), `C:\Windows`), "notepad.exe")
	return exec.Command(notepad, path).Start()
}

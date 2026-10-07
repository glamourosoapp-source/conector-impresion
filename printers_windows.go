//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

/*
Impresoras de Windows por la API del spooler (winspool.drv), sin PowerShell.

La versión de Bun listaba con `Get-Printer`, que no existe en Windows 7 y que
depende de que PowerShell no esté bloqueado: en Chapala la caja no veía ninguna
impresora. `EnumPrinters` está desde Windows 2000.

Los tickets se escriben en RAW (`StartDocPrinter` con `DATATYPE = "RAW"`): los
comandos ESC/POS deben llegar tal cual. `Out-Printer` o imprimir como texto hace
que el driver los reinterprete y el ticket sale con basura y sin corte.
*/

var (
	// Ruta completa: cargar "winspool.drv" por nombre buscaría primero en la
	// carpeta del .exe.
	winspool = syscall.NewLazyDLL(filepath.Join(firstNonEmpty(os.Getenv("SystemRoot"), `C:\Windows`), "System32", "winspool.drv"))

	procEnumPrinters      = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinter = winspool.NewProc("GetDefaultPrinterW")
	procOpenPrinter       = winspool.NewProc("OpenPrinterW")
	procClosePrinter      = winspool.NewProc("ClosePrinter")
	procStartDocPrinter   = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter     = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter  = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter    = winspool.NewProc("EndPagePrinter")
	procWritePrinter      = winspool.NewProc("WritePrinter")
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
	errInsufficientBuffer  = syscall.Errno(122)
)

// PRINTER_INFO_2W: todos los punteros miden lo que mide un puntero, así que la
// misma estructura sirve compilada a 32 o 64 bits.
type printerInfo2 struct {
	serverName         *uint16
	printerName        *uint16
	shareName          *uint16
	portName           *uint16
	driverName         *uint16
	comment            *uint16
	location           *uint16
	devMode            uintptr
	sepFile            *uint16
	printProcessor     *uint16
	datatype           *uint16
	parameters         *uint16
	securityDescriptor uintptr
	attributes         uint32
	priority           uint32
	defaultPriority    uint32
	startTime          uint32
	untilTime          uint32
	status             uint32
	jobs               uint32
	averagePPM         uint32
}

// PRINTER_INFO_4W: solo nombre, servidor y atributos. Es la consulta rápida
// que no toca las impresoras de red; se usa si la de nivel 2 falla.
type printerInfo4 struct {
	printerName *uint16
	serverName  *uint16
	attributes  uint32
}

// DOC_INFO_1W
type docInfo1 struct {
	docName    *uint16
	outputFile *uint16
	datatype   *uint16
}

func platformPrinters() printerBackend {
	return printerBackend{list: listWindowsPrinters, print: printRawWindows}
}

func enumPrinters(level uint32) ([]byte, uint32, error) {
	flags := uint32(printerEnumLocal | printerEnumConnections)
	var needed, returned uint32
	r1, _, err := procEnumPrinters.Call(uintptr(flags), 0, uintptr(level), 0, 0,
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r1 == 0 && err != errInsufficientBuffer {
		return nil, 0, fmt.Errorf("EnumPrinters: %v", err)
	}
	if needed == 0 {
		return nil, 0, nil
	}
	buf := make([]byte, needed)
	r1, _, err = procEnumPrinters.Call(uintptr(flags), 0, uintptr(level),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r1 == 0 {
		return nil, 0, fmt.Errorf("EnumPrinters: %v", err)
	}
	return buf, returned, nil
}

func listWindowsPrinters() ([]printerInfo, error) {
	defaultName := defaultPrinterName()

	buf, count, err := enumPrinters(2)
	if err == nil {
		printers := make([]printerInfo, 0, count)
		if count > 0 {
			entries := unsafe.Slice((*printerInfo2)(unsafe.Pointer(&buf[0])), count)
			for _, entry := range entries {
				name := utf16PtrToString(entry.printerName)
				if name == "" {
					continue
				}
				printers = append(printers, printerInfo{
					Name:      name,
					Driver:    utf16PtrToString(entry.driverName),
					PortName:  utf16PtrToString(entry.portName),
					IsDefault: strings.EqualFold(name, defaultName),
					Status:    describeStatus(entry.status),
				})
			}
		}
		return printers, nil
	}

	buf, count, fallbackErr := enumPrinters(4)
	if fallbackErr != nil {
		return nil, fmt.Errorf("No se pudo leer la lista de impresoras (%v). Revisa que el servicio Cola de impresión esté iniciado", err)
	}
	printers := make([]printerInfo, 0, count)
	if count > 0 {
		entries := unsafe.Slice((*printerInfo4)(unsafe.Pointer(&buf[0])), count)
		for _, entry := range entries {
			name := utf16PtrToString(entry.printerName)
			if name == "" {
				continue
			}
			printers = append(printers, printerInfo{Name: name, IsDefault: strings.EqualFold(name, defaultName)})
		}
	}
	return printers, nil
}

func defaultPrinterName() string {
	var size uint32
	procGetDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return ""
	}
	buf := make([]uint16, size)
	r1, _, _ := procGetDefaultPrinter.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r1 == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// Estado en palabras para la pantalla de configuración de la caja.
func describeStatus(status uint32) string {
	switch {
	case status == 0:
		return "Lista"
	case status&0x00000080 != 0:
		return "Sin conexión"
	case status&0x00000010 != 0:
		return "Sin papel"
	case status&0x00000008 != 0:
		return "Papel atorado"
	case status&0x00400000 != 0:
		return "Tapa abierta"
	case status&0x00000001 != 0:
		return "En pausa"
	case status&0x00000002 != 0:
		return "Con error"
	default:
		return fmt.Sprintf("Estado 0x%X", status)
	}
}

func printRawWindows(printerName string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("El ticket está vacío")
	}
	name, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return err
	}

	var handle syscall.Handle
	r1, _, callErr := procOpenPrinter.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&handle)), 0)
	if r1 == 0 {
		return fmt.Errorf("No se pudo abrir la impresora %q: %v", printerName, callErr)
	}
	defer procClosePrinter.Call(uintptr(handle))

	docName, _ := syscall.UTF16PtrFromString("Ticket Glamouroso")
	datatype, _ := syscall.UTF16PtrFromString("RAW")
	doc := docInfo1{docName: docName, datatype: datatype}
	r1, _, callErr = procStartDocPrinter.Call(uintptr(handle), 1, uintptr(unsafe.Pointer(&doc)))
	if r1 == 0 {
		return fmt.Errorf("StartDocPrinter falló: %v", callErr)
	}
	defer procEndDocPrinter.Call(uintptr(handle))

	r1, _, callErr = procStartPagePrinter.Call(uintptr(handle))
	if r1 == 0 {
		return fmt.Errorf("StartPagePrinter falló: %v", callErr)
	}
	defer procEndPagePrinter.Call(uintptr(handle))

	for offset := 0; offset < len(data); {
		var written uint32
		chunk := data[offset:]
		r1, _, callErr = procWritePrinter.Call(uintptr(handle),
			uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)),
			uintptr(unsafe.Pointer(&written)))
		if r1 == 0 || written == 0 {
			return fmt.Errorf("WritePrinter falló: %v", callErr)
		}
		offset += int(written)
	}
	return nil
}

func utf16PtrToString(ptr *uint16) string {
	if ptr == nil {
		return ""
	}
	// Se busca el terminador sin pasarse: las cadenas viven dentro del búfer
	// que llenó EnumPrinters.
	length := 0
	for p := unsafe.Pointer(ptr); *(*uint16)(p) != 0; p = unsafe.Add(p, 2) {
		length++
	}
	return syscall.UTF16ToString(unsafe.Slice(ptr, length))
}

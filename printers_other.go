//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

/*
En macOS y Linux el conector usa lpstat/lp solo para poder desarrollarlo y
probarlo fuera de Windows. Las sucursales son PC con Windows.
*/

func platformPrinters() printerBackend {
	return printerBackend{list: listCupsPrinters, print: printRawCups}
}

func listCupsPrinters() ([]printerInfo, error) {
	out, err := exec.Command("lpstat", "-p").Output()
	if err != nil {
		return []printerInfo{}, nil
	}
	defaultName := ""
	if raw, err := exec.Command("lpstat", "-d").Output(); err == nil {
		if idx := strings.LastIndex(string(raw), ":"); idx >= 0 {
			defaultName = strings.TrimSpace(string(raw)[idx+1:])
		}
	}
	var printers []printerInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "printer" {
			printers = append(printers, printerInfo{Name: fields[1], IsDefault: fields[1] == defaultName})
		}
	}
	return printers, nil
}

func printRawCups(printerName string, data []byte) error {
	cmd := exec.Command("lp", "-d", printerName, "-o", "raw")
	cmd.Stdin = bytes.NewReader(data)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lp falló: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

/**
 * Marca el .exe del conector como aplicación de ventana (subsistema GUI de
 * Windows) en vez de consola, para que al arrancar no abra la ventana negra.
 *
 * Es lo mismo que hace `bun build --windows-hide-console`, pero ese flag solo
 * funciona compilando *en* Windows, y el conector también se compila desde Mac.
 * Solo cambia un campo de la cabecera PE; el código y el runtime no se tocan.
 */
import { readFileSync, writeFileSync } from "node:fs";

const SUBSYSTEM_GUI = 2;
const SUBSYSTEM_CONSOLE = 3;

const path = process.argv[2];
if (!path) {
  console.error("Uso: bun scripts/hide-console.ts <ruta del .exe>");
  process.exit(1);
}

const exe = readFileSync(path);
const peOffset = exe.readUInt32LE(0x3c);
if (exe.toString("latin1", peOffset, peOffset + 4) !== "PE\0\0") {
  console.error(`${path} no es un ejecutable de Windows`);
  process.exit(1);
}

// Firma PE (4 bytes) + cabecera COFF (20) + campo Subsystem del encabezado
// opcional (offset 68, igual en PE32 y PE32+).
const subsystemOffset = peOffset + 4 + 20 + 68;
const current = exe.readUInt16LE(subsystemOffset);
if (current !== SUBSYSTEM_CONSOLE && current !== SUBSYSTEM_GUI) {
  console.error(`Subsistema inesperado (${current}); no se modificó ${path}`);
  process.exit(1);
}

exe.writeUInt16LE(SUBSYSTEM_GUI, subsystemOffset);
writeFileSync(path, exe);
console.log(`${path}: sin ventana de consola (subsistema ${current} → ${SUBSYSTEM_GUI})`);

import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

/**
 * Respaldo de la cola de la caja.
 *
 * El navegador guarda las ventas cobradas en IndexedDB, que vive dentro del
 * perfil de Chrome: basta que alguien "limpie el navegador" para llevarse
 * ventas que nunca llegaron al servidor. Este agente ya corre en la misma PC
 * para imprimir, así que es el único lugar donde la caja puede dejar una copia
 * sin instalar nada más.
 *
 * El agente **no sube nada al servidor**: no tiene credenciales y no se las
 * vamos a dar (subir desde aquí se evalúa después del piloto). Es un espejo
 * para restaurar, y nada más.
 */

const MAX_BYTES = 8 * 1024 * 1024;

export interface BackupPayload {
  outbox: unknown[];
  counter: unknown;
  meta: unknown;
  savedAt: string;
}

function backupPath(dataDir: string): string {
  return join(dataDir, "pos-backup.json");
}

/**
 * Escribe el respaldo de forma atómica: primero un archivo temporal y luego un
 * rename. Escribir encima del bueno deja el respaldo a medias si la PC se apaga
 * justo ahí, que es exactamente el escenario del que protege.
 */
export function writeBackup(dataDir: string, payload: BackupPayload): void {
  const target = backupPath(dataDir);
  mkdirSync(dirname(target), { recursive: true });
  const temp = `${target}.tmp`;
  writeFileSync(temp, JSON.stringify(payload), { encoding: "utf8" });
  renameSync(temp, target);
}

export function readBackup(dataDir: string): BackupPayload | null {
  const target = backupPath(dataDir);
  if (!existsSync(target)) return null;
  try {
    return JSON.parse(readFileSync(target, "utf8")) as BackupPayload;
  } catch {
    // Un respaldo corrupto se ignora: la caja arranca vacía, que es mejor que
    // arrancar con datos a medias que nadie puede auditar.
    return null;
  }
}

/** Un respaldo absurdamente grande es un error, no una sucursal muy trabajadora. */
export function isTooLarge(raw: string): boolean {
  return raw.length > MAX_BYTES;
}

import { mkdirSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { randomBytes } from "node:crypto";

/**
 * Configuración del conector. El token se genera la primera vez y se guarda en
 * disco: es lo que el cajero copia una sola vez en la pantalla de configuración
 * del POS para emparejar esa caja con esta impresora.
 */

const DEFAULT_PORT = 9377;

function dataDir(): string {
  const base =
    process.env.GLAM_AGENT_DATA_DIR ||
    process.env.ProgramData ||
    process.env.APPDATA ||
    process.cwd();
  return join(base, "GlamourosoPrintAgent");
}

export interface AgentConfig {
  port: number;
  token: string;
  /** Orígenes del POS autorizados a hablarle al conector. */
  allowedOrigins: string[];
  logPath: string;
  /** Carpeta del conector: ahí vive el respaldo de la cola de la caja. */
  dataDir: string;
}

export function loadConfig(): AgentConfig {
  const dir = dataDir();
  mkdirSync(dir, { recursive: true });

  const tokenPath = join(dir, "token.txt");
  let token: string;
  if (existsSync(tokenPath)) {
    token = readFileSync(tokenPath, "utf8").trim();
  } else {
    // "wx": el instalador arranca el conector y "Ver token" casi al mismo tiempo;
    // si los dos generaran uno, la caja quedaría emparejada con el que se perdió.
    token = randomBytes(16).toString("hex");
    try {
      writeFileSync(tokenPath, token, { encoding: "utf8", flag: "wx" });
    } catch {
      token = readFileSync(tokenPath, "utf8").trim();
    }
  }

  const allowedOrigins = (
    process.env.GLAM_ALLOWED_ORIGINS ||
    "http://localhost:3000,https://glamouroso.app"
  )
    .split(",")
    .map((origin) => origin.trim())
    .filter(Boolean);

  return {
    port: Number(process.env.GLAM_AGENT_PORT || DEFAULT_PORT),
    token,
    allowedOrigins,
    logPath: join(dir, "agent.log"),
    dataDir: dir,
  };
}

import { appendFileSync } from "node:fs";
import { loadConfig, type AgentConfig } from "./config";
import { agentHostname, listPrinters, printRaw } from "./printers";
import { isTooLarge, readBackup, writeBackup } from "./backup";

/**
 * Agente de impresión del punto de venta de Glamouroso.
 *
 * Corre en la PC de la sucursal y **solo escucha en loopback** (127.0.0.1), así
 * que nada de la red local puede hablarle. El POS (una página https) sí puede:
 * Chrome trata loopback como origen potencialmente confiable y no lo bloquea
 * como contenido mixto.
 *
 * Cada petición exige el token que se generó en la primera ejecución, y el
 * origen debe estar en la lista blanca. Sin eso, cualquier página abierta en esa
 * PC podría mandar a imprimir.
 */

const VERSION = "1.0.0";
const config = loadConfig();

function log(message: string, extra?: Record<string, unknown>): void {
  const line = `${new Date().toISOString()} ${message}${extra ? ` ${JSON.stringify(extra)}` : ""}`;
  console.log(line);
  try {
    appendFileSync(config.logPath, `${line}\n`);
  } catch {
    /* sin permiso de escritura el agente sigue imprimiendo */
  }
}

function corsHeaders(origin: string | null, agentConfig: AgentConfig): Record<string, string> {
  const allowed = origin && agentConfig.allowedOrigins.includes(origin) ? origin : "";
  return {
    "Access-Control-Allow-Origin": allowed,
    "Access-Control-Allow-Headers": "Content-Type, Authorization",
    "Access-Control-Allow-Methods": "GET, POST, PUT, OPTIONS",
    "Access-Control-Max-Age": "600",
    Vary: "Origin",
  };
}

function json(body: unknown, status: number, headers: Record<string, string>): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { ...headers, "Content-Type": "application/json" },
  });
}

function authorized(request: Request): boolean {
  const header = request.headers.get("authorization") ?? "";
  return header === `Bearer ${config.token}`;
}

const server = Bun.serve({
  // Loopback a propósito: el agente no se expone a la red de la sucursal.
  hostname: "127.0.0.1",
  port: config.port,

  async fetch(request) {
    const url = new URL(request.url);
    const origin = request.headers.get("origin");
    const headers = corsHeaders(origin, config);

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers });
    }

    // El health check no pide token: el POS lo usa para saber si hay agente y
    // no revela nada (nombre, versión y equipo).
    if (url.pathname === "/health") {
      return json({ name: "glamouroso-print-agent", version: VERSION, hostname: agentHostname() }, 200, headers);
    }

    if (!authorized(request)) {
      return json({ error: "Token inválido. Cópialo de la ventana del agente." }, 401, headers);
    }

    if (url.pathname === "/printers" && request.method === "GET") {
      try {
        const printers = await listPrinters();
        return json({ printers }, 200, headers);
      } catch (error) {
        log("listPrinters falló", { error: (error as Error).message });
        return json({ error: (error as Error).message }, 500, headers);
      }
    }

    if (url.pathname === "/print" && request.method === "POST") {
      try {
        const body = (await request.json()) as { printerName?: string; dataBase64?: string };
        if (!body.printerName || !body.dataBase64) {
          return json({ error: "Faltan printerName o dataBase64" }, 400, headers);
        }
        const bytes = Uint8Array.from(atob(body.dataBase64), (char) => char.charCodeAt(0));
        await printRaw(body.printerName, bytes);
        log("Ticket impreso", { printer: body.printerName, bytes: bytes.length });
        return json({ ok: true, bytes: bytes.length }, 200, headers);
      } catch (error) {
        log("print falló", { error: (error as Error).message });
        return json({ error: (error as Error).message }, 500, headers);
      }
    }

    /**
     * Respaldo de la cola de la caja.
     *
     * Solo guarda y devuelve: el agente nunca habla con el servidor de
     * Glamouroso. Si la PC pierde el perfil de Chrome, esto es lo que permite
     * recuperar las ventas cobradas que no habían subido.
     */
    if (url.pathname === "/pos/backup" && request.method === "PUT") {
      try {
        const raw = await request.text();
        if (isTooLarge(raw)) {
          return json({ error: "El respaldo es demasiado grande" }, 413, headers);
        }
        const payload = JSON.parse(raw) as Parameters<typeof writeBackup>[1];
        if (!payload || !Array.isArray(payload.outbox)) {
          return json({ error: "Respaldo inválido" }, 400, headers);
        }
        writeBackup(config.dataDir, payload);
        return json({ ok: true, pending: payload.outbox.length }, 200, headers);
      } catch (error) {
        log("respaldo falló", { error: (error as Error).message });
        return json({ error: (error as Error).message }, 500, headers);
      }
    }

    if (url.pathname === "/pos/backup" && request.method === "GET") {
      const payload = readBackup(config.dataDir);
      if (!payload) return json({ outbox: [], counter: null, meta: null, savedAt: null }, 200, headers);
      return json(payload, 200, headers);
    }

    return json({ error: "Ruta no encontrada" }, 404, headers);
  },
});

log(`Agente de impresión escuchando en http://127.0.0.1:${server.port}`);
log(`Token de emparejamiento: ${config.token}`);
log(`Orígenes permitidos: ${config.allowedOrigins.join(", ")}`);
console.log("");
console.log("================================================================");
console.log("  Glamouroso · Agente de impresión");
console.log("  Deja esta ventana abierta mientras uses la caja.");
console.log("");
console.log(`  Token para la configuración del POS:  ${config.token}`);
console.log("================================================================");

import { hostname } from "node:os";

/**
 * Lista e imprime en las impresoras de Windows.
 *
 * Se usa PowerShell en vez de una librería nativa para que el agente compile a
 * un solo `.exe` sin dependencias: `Get-Printer` para listar y la API
 * `winspool.drv` para escribir RAW (los comandos ESC/POS deben llegar tal cual,
 * sin que el driver los interprete como texto).
 */

export interface PrinterInfo {
  name: string;
  driver?: string;
  portName?: string;
  isDefault?: boolean;
  status?: string;
}

const IS_WINDOWS = process.platform === "win32";

async function powershell(script: string): Promise<string> {
  const proc = Bun.spawn(
    ["powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script],
    { stdout: "pipe", stderr: "pipe" }
  );
  const [out, err, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  if (code !== 0) throw new Error(err.trim() || `PowerShell terminó con código ${code}`);
  return out;
}

export async function listPrinters(): Promise<PrinterInfo[]> {
  if (!IS_WINDOWS) {
    // En macOS/Linux (desarrollo) se usa lpstat, para poder probar el agente.
    try {
      const proc = Bun.spawn(["lpstat", "-p"], { stdout: "pipe", stderr: "pipe" });
      const out = await new Response(proc.stdout).text();
      await proc.exited;
      return out
        .split("\n")
        .map((line) => line.match(/^printer (\S+)/)?.[1])
        .filter((name): name is string => Boolean(name))
        .map((name) => ({ name }));
    } catch {
      return [];
    }
  }

  const json = await powershell(
    "Get-Printer | Select-Object Name,DriverName,PortName,PrinterStatus | ConvertTo-Json -Compress"
  );
  const parsed = JSON.parse(json || "[]");
  const rows: Array<Record<string, unknown>> = Array.isArray(parsed) ? parsed : [parsed];

  const defaultName = await powershell(
    "(Get-CimInstance -Class Win32_Printer | Where-Object { $_.Default -eq $true }).Name"
  ).catch(() => "");

  return rows
    .filter((row) => row && row.Name)
    .map((row) => ({
      name: String(row.Name),
      driver: row.DriverName ? String(row.DriverName) : undefined,
      portName: row.PortName ? String(row.PortName) : undefined,
      status: row.PrinterStatus != null ? String(row.PrinterStatus) : undefined,
      isDefault: String(row.Name) === defaultName.trim(),
    }));
}

/**
 * Escribe bytes RAW a una impresora por nombre.
 *
 * `Out-Printer` no sirve: manda texto y el driver reinterpreta los comandos
 * ESC/POS (el ticket sale con basura y sin corte de papel). La vía correcta es
 * `OpenPrinter`/`StartDocPrinter` con `DATATYPE = "RAW"`.
 */
export async function printRaw(printerName: string, data: Uint8Array): Promise<void> {
  if (!IS_WINDOWS) {
    const proc = Bun.spawn(["lp", "-d", printerName, "-o", "raw"], { stdin: "pipe", stderr: "pipe" });
    proc.stdin.write(data);
    proc.stdin.end();
    const code = await proc.exited;
    if (code !== 0) throw new Error(`lp terminó con código ${code}`);
    return;
  }

  const temp = `${process.env.TEMP || "C:\\Windows\\Temp"}\\glam-ticket-${Date.now()}.bin`;
  await Bun.write(temp, data);

  const script = `
$ErrorActionPreference = "Stop"
$signature = @'
using System;
using System.IO;
using System.Runtime.InteropServices;
public class RawPrinter {
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  public class DOCINFO { public string pDocName; public string pOutputFile; public string pDataType; }
  [DllImport("winspool.drv", CharSet = CharSet.Unicode, SetLastError = true)]
  public static extern bool OpenPrinter(string src, out IntPtr hPrinter, IntPtr pd);
  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool ClosePrinter(IntPtr hPrinter);
  [DllImport("winspool.drv", CharSet = CharSet.Unicode, SetLastError = true)]
  public static extern bool StartDocPrinter(IntPtr hPrinter, int level, [In, MarshalAs(UnmanagedType.LPStruct)] DOCINFO di);
  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool EndDocPrinter(IntPtr hPrinter);
  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool StartPagePrinter(IntPtr hPrinter);
  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool EndPagePrinter(IntPtr hPrinter);
  [DllImport("winspool.drv", SetLastError = true)]
  public static extern bool WritePrinter(IntPtr hPrinter, IntPtr pBytes, int dwCount, out int dwWritten);

  public static void SendFile(string printer, string file) {
    byte[] bytes = File.ReadAllBytes(file);
    IntPtr hPrinter;
    if (!OpenPrinter(printer, out hPrinter, IntPtr.Zero)) throw new Exception("No se pudo abrir la impresora " + printer);
    try {
      DOCINFO di = new DOCINFO();
      di.pDocName = "Ticket Glamouroso";
      di.pDataType = "RAW";
      if (!StartDocPrinter(hPrinter, 1, di)) throw new Exception("StartDocPrinter falló");
      try {
        if (!StartPagePrinter(hPrinter)) throw new Exception("StartPagePrinter falló");
        IntPtr buffer = Marshal.AllocCoTaskMem(bytes.Length);
        try {
          Marshal.Copy(bytes, 0, buffer, bytes.Length);
          int written;
          if (!WritePrinter(hPrinter, buffer, bytes.Length, out written)) throw new Exception("WritePrinter falló");
        } finally { Marshal.FreeCoTaskMem(buffer); }
        EndPagePrinter(hPrinter);
      } finally { EndDocPrinter(hPrinter); }
    } finally { ClosePrinter(hPrinter); }
  }
}
'@
Add-Type -TypeDefinition $signature -Language CSharp
[RawPrinter]::SendFile(${JSON.stringify(printerName)}, ${JSON.stringify(temp)})
`;

  try {
    await powershell(script);
  } finally {
    await Bun.file(temp)
      .delete()
      .catch(() => null);
  }
}

export function agentHostname(): string {
  return hostname();
}

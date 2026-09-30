# Glamouroso · Conector de impresión

Servicio local que imprime los tickets del punto de venta en la impresora térmica de la sucursal **sin el diálogo del navegador**.

## Por qué existe

El POS corre en Chrome. Un navegador no puede elegir impresora ni imprimir en silencio, y tampoco puede mandar comandos ESC/POS crudos (ancho de papel, negritas, corte de papel). Este conector sí: recibe el ticket ya armado desde la caja y lo escribe en RAW a la impresora de Windows.

Sin el conector, el POS sigue funcionando: al cobrar con **F1** se abre el diálogo de impresión del navegador con el ticket listo. El conector solo quita ese toque extra.

## Se llamaba "agente de impresión"

Cambió a **Conector de impresión** el 2026-09-19. En el CRM "Agente IA" ya es el agente de WhatsApp, y en una llamada con una sucursal "no jala el agente" no puede significar dos cosas.

Lo que **no** cambió, a propósito: la carpeta del repo (`PrintAgent/`), los identificadores del código (`PrintAgentConfig`, `detectPrintAgent`), la clave de `localStorage` (`pos.printAgent`) y la carpeta de datos (`GlamourosoPrintAgent`). Renombrar la clave desemparejaría todas las cajas ya configuradas, y renombrar la carpeta dejaría huérfanos el token y el respaldo de la cola. Cambió el nombre del producto, no el de las llaves.


## Publicar una versión

El instalador vive en las **publicaciones de GitHub** de este repo, no en un bucket: la dirección de la última versión no cambia nunca, GitHub la sirve como archivo adjunto (que es lo que hace que el navegador la descargue en vez de abrirla) y no hay nada que mantener ni pagar. La caja apunta ahí desde `NEXT_PUBLIC_CONNECTOR_INSTALLER_URL`, con ese valor por defecto:

```
https://github.com/glamourosoapp-source/conector-impresion/releases/latest/download/setup-conector-impresion.exe
```

### Lo normal: que lo compile GitHub

Subir una etiqueta de versión basta. El workflow `.github/workflows/release.yml` corre en una máquina Windows de GitHub, compila el `.exe`, arma el instalador con Inno Setup, lo renombra a `setup-conector-impresion.exe` y crea la publicación:

```bash
git tag v1.1.0
git push origin v1.1.0
```

Antes de etiquetar, sube la versión en los tres lugares: `package.json`, `VERSION` en `src/server.ts` y `MyAppVersion` en `installer/setup.iss`.

### A mano, si GitHub Actions no está disponible

El ejecutable se compila en cualquier sistema, pero **el instalador necesita Windows**: Inno Setup no tiene versión para macOS ni Linux.

### 1. El ejecutable (desde cualquier máquina)

```bash
bun run build:win
```

Deja `dist/glamouroso-print-agent.exe` (~94 MB, Bun compila su runtime dentro).

### 2. El instalador (en una PC con Windows)

Requiere [Inno Setup](https://jrsoftware.org/isdl.php). Con el repo clonado y el `.exe` ya en `dist\`:

```
iscc installer\setup.iss
```

Sale `installer\Output\setup-glamouroso-print-agent.exe`. **Renómbralo a `setup-conector-impresion.exe`**, que es el nombre que espera la caja.

### 3. Publicarlo

```bash
gh release create v1.1.0 setup-conector-impresion.exe \
  --title "Conector de impresión 1.1.0" \
  --notes "Respaldo de la cola de la caja y cambio de nombre."
```

Desde ese momento el botón **Descargar el instalador** de `/pos/configuracion` baja esta versión, sin tocar el Front.

### Sobre el aviso de Windows

El ejecutable **no está firmado**, así que al abrirlo Windows muestra "Windows protegió su PC" y hay que dar clic en *Más información* → *Ejecutar de todos modos*. La pantalla de configuración de la caja ya lo explica paso a paso, con esas mismas palabras, para que quien instale no se asuste.

Firmarlo quitaría el aviso, pero cuesta (certificado anual) y la reputación con SmartScreen tarda meses en construirse. Como el aviso lo ve **una sola vez quien monta la PC**, y nunca el cajero en su día a día, no se justifica todavía. Si en algún momento las sucursales se instalan solas, hay que reconsiderarlo.

## Seguridad

- Escucha **solo en `127.0.0.1`**: nada de la red de la sucursal puede hablarle.
- Cada petición (salvo `/health`) exige el **token** que se genera en la primera ejecución y se guarda en `%ProgramData%\GlamourosoPrintAgent\token.txt`. La ventana del conector lo muestra al arrancar.
- El origen del POS debe estar en la lista blanca (`GLAM_ALLOWED_ORIGINS`).

## Instalación en una sucursal

1. Correr el instalador `setup-glamouroso-print-agent.exe`. En el asistente, dejar marcada la tarea **Crear el acceso directo del Punto de venta** y confirmar la dirección de la caja.
2. Abrir el conector. La ventana muestra el token.
3. En la caja, entrar a **Configuración** (`/pos/configuracion`), pegar el token, elegir la impresora de la lista y presionar **Imprimir prueba**.
4. El instalador deja el conector en el inicio de sesión, así que arranca solo con la PC.

## El acceso directo del Punto de venta

El instalador hace dos cosas en la PC de la sucursal, no una: instala el conector y deja en el escritorio el icono **Glamouroso Punto de venta**, para que el cajero abra la caja con un clic en el logo y no escribiendo una dirección.

El acceso directo apunta al navegador en modo aplicación —`chrome.exe --app=<url de la caja>`— con `glamouroso-pos.ico` como icono, así que abre una ventana propia sin barra de direcciones ni pestañas. Busca **Chrome** en el registro (`App Paths`) y, si no está, **Edge**, que viene en todo Windows 10/11; sin ninguno de los dos avisa y no crea un acceso directo roto.

`installer/glamouroso-pos.ico` **no se edita a mano**: lo genera `bun run icons:pos` en `Front/` a partir de la G de `Front/src/app/icon.svg` y lo copia aquí, para que el icono del escritorio y el de la app instalada sean el mismo trazo.

Es una alternativa a instalar la PWA desde la propia caja (botón **Instalar en el escritorio**): cualquiera de las dos deja el icono, y esta no depende de que alguien se acuerde de hacerlo en cada sucursal.

**Solo Windows.** El instalador es Inno Setup y no tiene equivalente para macOS. En una Mac la caja se instala igual de bien, pero por el botón de la propia caja: Chrome deja la app en Launchpad y se arrastra al Dock. El Conector de impresión sí corre en macOS (usa `lpstat`/`lp`), pero eso está ahí **para poder desarrollarlo y probarlo fuera de Windows**: no hay build compilado ni paquete para Mac, y no se ha validado contra una impresora térmica real en ese sistema. Las sucursales son PC con Windows.

## API

| Método | Ruta | Para qué |
|---|---|---|
| GET | `/health` | Saber si hay conector (sin token). Devuelve nombre, versión y equipo. |
| GET | `/printers` | Impresoras instaladas en esa PC, con la predeterminada marcada. |
| POST | `/print` | `{ printerName, dataBase64 }`: escribe esos bytes en RAW. |

## Desarrollo

```bash
bun install
bun run dev          # levanta en http://127.0.0.1:9377
bun run typecheck
bun run build:win    # dist/glamouroso-print-agent.exe
```

En macOS y Linux el conector usa `lpstat`/`lp` para poder probarse fuera de Windows; en producción usa `Get-Printer` y la API `winspool.drv` (`OpenPrinter` + `StartDocPrinter` con `DATATYPE = "RAW"`).

**Por qué no `Out-Printer`**: manda el contenido como texto y el driver reinterpreta los comandos ESC/POS. El ticket sale con basura y sin corte de papel.

## Variables

| Variable | Default | Para qué |
|---|---|---|
| `GLAM_AGENT_PORT` | `9377` | Puerto de loopback. |
| `GLAM_ALLOWED_ORIGINS` | `http://localhost:3000,https://glamouroso.app` | Orígenes del POS autorizados. |
| `GLAM_AGENT_DATA_DIR` | `%ProgramData%` | Dónde viven el token y el log. |

## Problemas comunes

- **El POS dice que no detecta el conector**: revisar que la ventana esté abierta y que el puerto coincida con el de la configuración de la caja.
- **Token inválido**: el token se regenera si se borra `token.txt`. Volver a copiarlo en la configuración del POS.
- **El ticket sale con caracteres raros**: la impresora no está en la tabla CP850. Se puede cambiar en `escpos.ts` del Front (`ESC t`).
- **No corta el papel**: la impresora no soporta el corte parcial (`GS V 66`). No impide imprimir.

## Respaldo de la cola de la caja

Desde 2026-09-18 el conector también guarda una copia de lo que la caja tiene sin subir.

La caja cobra sin internet y guarda las ventas en IndexedDB, que vive dentro del perfil de Chrome: basta que alguien "limpie el navegador" para llevarse ventas que nunca llegaron al servidor. El conector ya corre en la misma PC, así que es el único lugar donde dejar una copia sin instalar nada más.

| Método | Ruta | Qué hace |
|---|---|---|
| `PUT` | `/pos/backup` | Guarda la cola, el consecutivo del folio y los metadatos de la caja |
| `GET` | `/pos/backup` | Devuelve el último respaldo, o vacío si no hay |

Las dos piden el mismo token que `/print`. El archivo (`pos-backup.json`, en la carpeta de datos del conector) se escribe de forma **atómica**: primero un temporal y luego un rename, porque escribir encima del bueno lo deja a medias si la PC se apaga justo ahí, que es exactamente el escenario del que protege. Un respaldo corrupto se ignora al leerlo.

**El conector no habla con el servidor de Glamouroso.** No tiene credenciales y no se las vamos a dar: esto es un espejo para restaurar, no un segundo camino de subida. Subir desde el conector con el navegador cerrado se evaluará después del piloto.

La caja restaura sola: si al arrancar su base local está vacía y el conector tiene un respaldo con eventos, los vuelve a encolar y avisa en pantalla.

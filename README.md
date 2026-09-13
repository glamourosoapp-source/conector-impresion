# Glamouroso · Agente de impresión

Servicio local que imprime los tickets del punto de venta en la impresora térmica de la sucursal **sin el diálogo del navegador**.

## Por qué existe

El POS corre en Chrome. Un navegador no puede elegir impresora ni imprimir en silencio, y tampoco puede mandar comandos ESC/POS crudos (ancho de papel, negritas, corte de papel). Este agente sí: recibe el ticket ya armado desde la caja y lo escribe en RAW a la impresora de Windows.

Sin el agente, el POS sigue funcionando: al cobrar con **F1** se abre el diálogo de impresión del navegador con el ticket listo. El agente solo quita ese toque extra.

## Seguridad

- Escucha **solo en `127.0.0.1`**: nada de la red de la sucursal puede hablarle.
- Cada petición (salvo `/health`) exige el **token** que se genera en la primera ejecución y se guarda en `%ProgramData%\GlamourosoPrintAgent\token.txt`. La ventana del agente lo muestra al arrancar.
- El origen del POS debe estar en la lista blanca (`GLAM_ALLOWED_ORIGINS`).

## Instalación en una sucursal

1. Correr el instalador `setup-glamouroso-print-agent.exe`. En el asistente, dejar marcada la tarea **Crear el acceso directo del Punto de venta** y confirmar la dirección de la caja.
2. Abrir el agente. La ventana muestra el token.
3. En la caja, entrar a **Configuración** (`/pos/configuracion`), pegar el token, elegir la impresora de la lista y presionar **Imprimir prueba**.
4. El instalador deja el agente en el inicio de sesión, así que arranca solo con la PC.

## El acceso directo del Punto de venta

El instalador hace dos cosas en la PC de la sucursal, no una: instala el agente y deja en el escritorio el icono **Glamouroso Punto de venta**, para que el cajero abra la caja con un clic en el logo y no escribiendo una dirección.

El acceso directo apunta al navegador en modo aplicación —`chrome.exe --app=<url de la caja>`— con `glamouroso-pos.ico` como icono, así que abre una ventana propia sin barra de direcciones ni pestañas. Busca **Chrome** en el registro (`App Paths`) y, si no está, **Edge**, que viene en todo Windows 10/11; sin ninguno de los dos avisa y no crea un acceso directo roto.

`installer/glamouroso-pos.ico` **no se edita a mano**: lo genera `bun run icons:pos` en `Front/` a partir de la G de `Front/src/app/icon.svg` y lo copia aquí, para que el icono del escritorio y el de la app instalada sean el mismo trazo.

Es una alternativa a instalar la PWA desde la propia caja (botón **Instalar en el escritorio**): cualquiera de las dos deja el icono, y esta no depende de que alguien se acuerde de hacerlo en cada sucursal.

**Solo Windows.** El instalador es Inno Setup y no tiene equivalente para macOS. En una Mac la caja se instala igual de bien, pero por el botón de la propia caja: Chrome deja la app en Launchpad y se arrastra al Dock. El agente de impresión sí corre en macOS (usa `lpstat`/`lp`), pero eso está ahí **para poder desarrollarlo y probarlo fuera de Windows**: no hay build compilado ni paquete para Mac, y no se ha validado contra una impresora térmica real en ese sistema. Las sucursales son PC con Windows.

## API

| Método | Ruta | Para qué |
|---|---|---|
| GET | `/health` | Saber si hay agente (sin token). Devuelve nombre, versión y equipo. |
| GET | `/printers` | Impresoras instaladas en esa PC, con la predeterminada marcada. |
| POST | `/print` | `{ printerName, dataBase64 }`: escribe esos bytes en RAW. |

## Desarrollo

```bash
bun install
bun run dev          # levanta en http://127.0.0.1:9377
bun run typecheck
bun run build:win    # dist/glamouroso-print-agent.exe
```

En macOS y Linux el agente usa `lpstat`/`lp` para poder probarse fuera de Windows; en producción usa `Get-Printer` y la API `winspool.drv` (`OpenPrinter` + `StartDocPrinter` con `DATATYPE = "RAW"`).

**Por qué no `Out-Printer`**: manda el contenido como texto y el driver reinterpreta los comandos ESC/POS. El ticket sale con basura y sin corte de papel.

## Variables

| Variable | Default | Para qué |
|---|---|---|
| `GLAM_AGENT_PORT` | `9377` | Puerto de loopback. |
| `GLAM_ALLOWED_ORIGINS` | `http://localhost:3000,https://glamouroso.vercel.app` | Orígenes del POS autorizados. |
| `GLAM_AGENT_DATA_DIR` | `%ProgramData%` | Dónde viven el token y el log. |

## Problemas comunes

- **El POS dice que no detecta el agente**: revisar que la ventana esté abierta y que el puerto coincida con el de la configuración de la caja.
- **Token inválido**: el token se regenera si se borra `token.txt`. Volver a copiarlo en la configuración del POS.
- **El ticket sale con caracteres raros**: la impresora no está en la tabla CP850. Se puede cambiar en `escpos.ts` del Front (`ESC t`).
- **No corta el papel**: la impresora no soporta el corte parcial (`GS V 66`). No impide imprimir.

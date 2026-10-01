; Instalador de la PC de sucursal: conector de impresión + acceso directo a la caja.
; Compilar con: iscc installer\setup.iss  (requiere dist\glamouroso-print-agent.exe)

#define MyAppName "Glamouroso Conector de Impresion"
#define MyAppVersion "1.2.0"
#define MyAppPublisher "Anawim"
#define MyAppExeName "glamouroso-print-agent.exe"
#define PosShortcutName "Glamouroso Punto de venta"
#define TokenShortcutName "Ver token del Conector de impresion"
#define PosIcoName "glamouroso-pos.ico"
#define DefaultPosUrl "https://glamouroso.app/pos"

[Setup]
AppId={{8E2A4C11-7B3D-4F55-9C21-POS-GLAMOUROSO}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\Glamouroso\PrintAgent
DefaultGroupName=Glamouroso
DisableProgramGroupPage=yes
OutputBaseFilename=setup-glamouroso-print-agent
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Files]
Source: "..\dist\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion
; Icono de la caja: se genera desde la G de Glamouroso con `bun run icons:pos`
; en Front/ y se copia aquí. Es el que ve el cajero en el escritorio.
Source: "{#PosIcoName}"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "posshortcut"; Description: "Crear el acceso directo del Punto de venta en el escritorio"; GroupDescription: "Punto de venta:"

[Icons]
; El conector corre sin ventana (desde 1.2.0) y arranca solo con la sesión: el
; cajero no tiene nada que abrir, así que ya no va al escritorio. En el menú
; Inicio quedan "iniciarlo" (por si se cerró desde el Administrador de tareas;
; si ya corre, no hace nada) y "ver el token", para emparejar la caja.
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\{#TokenShortcutName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--mostrar-token"

; La caja se abre con el navegador en modo aplicación (`--app=`): ventana propia,
; sin barra de direcciones ni pestañas, con la G de Glamouroso como icono. Es lo
; mismo que deja "Instalar" dentro de la caja, pero sin depender de que alguien
; se acuerde de hacerlo en cada una de las 28 sucursales.
Name: "{autodesktop}\{#PosShortcutName}"; Filename: "{code:GetBrowserPath}"; \
  Parameters: "--app={code:GetPosUrl}"; IconFilename: "{app}\{#PosIcoName}"; \
  Comment: "Caja del punto de venta de Glamouroso"; Tasks: posshortcut; Check: HasBrowser
Name: "{group}\{#PosShortcutName}"; Filename: "{code:GetBrowserPath}"; \
  Parameters: "--app={code:GetPosUrl}"; IconFilename: "{app}\{#PosIcoName}"; \
  Tasks: posshortcut; Check: HasBrowser

[Run]
; Arranca el conector siempre (también en instalación silenciosa); no abre ventana.
Filename: "{app}\{#MyAppExeName}"; Flags: nowait runhidden
; Al terminar, el token en el Bloc de notas para pegarlo en la configuración de la caja.
Filename: "{app}\{#MyAppExeName}"; Parameters: "--mostrar-token"; Description: "Ver el token para emparejar la caja"; Flags: postinstall nowait skipifsilent

[UninstallRun]
; Sin ventana no hay cómo cerrarlo a mano: se detiene antes de borrar el .exe.
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#MyAppExeName}"; Flags: runhidden; RunOnceId: "DetenerConector"

[Registry]
; Arranque automático al iniciar sesión: la caja no debe depender de que
; alguien se acuerde de abrir el conector.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; \
  ValueName: "GlamourosoPrintAgent"; ValueData: """{app}\{#MyAppExeName}"""; Flags: uninsdeletevalue

[UninstallDelete]
Type: filesandordirs; Name: "{commonappdata}\GlamourosoPrintAgent"

[InstallDelete]
; Las versiones anteriores a 1.2.0 dejaban el conector en el escritorio.
Type: files; Name: "{autodesktop}\{#MyAppName}.lnk"

[Code]
var
  PosUrlPage: TInputQueryWizardPage;
  BrowserPath: String;

{ Chrome primero (es el navegador con el que se probó la caja); si no está, Edge,
  que viene en todo Windows 10/11. Ambos entienden `--app=`. }
function ResolveBrowser(): String;
var
  Path: String;
begin
  Result := '';
  if RegQueryStringValue(HKLM, 'SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe', '', Path) then
    Result := RemoveQuotes(Path)
  else if RegQueryStringValue(HKLM32, 'SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe', '', Path) then
    Result := RemoveQuotes(Path)
  else if RegQueryStringValue(HKLM, 'SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe', '', Path) then
    Result := RemoveQuotes(Path)
  else if RegQueryStringValue(HKLM32, 'SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe', '', Path) then
    Result := RemoveQuotes(Path);

  if (Result <> '') and not FileExists(Result) then
    Result := '';
end;

procedure InitializeWizard();
begin
  BrowserPath := ResolveBrowser();

  PosUrlPage := CreateInputQueryPage(wpSelectTasks,
    'Punto de venta',
    'Direccion de la caja',
    'Es la direccion que abrira el acceso directo del escritorio. Dejala como esta salvo que la oficina indique otra.');
  PosUrlPage.Add('Direccion:', False);
  PosUrlPage.Values[0] := '{#DefaultPosUrl}';
end;

function GetPosUrl(Param: String): String;
begin
  Result := Trim(PosUrlPage.Values[0]);
  if Result = '' then
    Result := '{#DefaultPosUrl}';
end;

function GetBrowserPath(Param: String): String;
begin
  Result := BrowserPath;
end;

{ Al actualizar, el conector viejo sigue corriendo (y sin ventana no hay cómo
  cerrarlo): se detiene para poder reemplazar el .exe. Al terminar se vuelve a
  arrancar desde [Run]. El token y el respaldo de la cola no se tocan. }
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#MyAppExeName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := '';
end;

function HasBrowser(): Boolean;
begin
  Result := BrowserPath <> '';
end;

{ Sin Chrome ni Edge no hay acceso directo posible: se avisa en vez de crear uno roto. }
function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if (CurPageID = PosUrlPage.ID) and not HasBrowser() and IsTaskSelected('posshortcut') then
  begin
    MsgBox('No se encontro Chrome ni Edge en esta computadora, asi que no se creara el acceso directo del Punto de venta.' + #13#10 +
           'Instala Chrome y vuelve a correr este instalador, o abre la caja desde el navegador y usa "Instalar en el escritorio".',
           mbInformation, MB_OK);
  end;
end;

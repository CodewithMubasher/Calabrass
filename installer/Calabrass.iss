; Calabrass — Inno Setup script.
;
; Do not compile this by hand: installer\build-installer.ps1 builds the desktop
; app, builds the two MCP servers, and then compiles this into
; installer\out\Calabrass-setup-v1.0.3-x64.exe.

#define AppName "Calabrass"
; The one place the version is written down. Bump it here, and the installer,
; its file name and the entry in Apps & features all follow. It can also be
; overridden for a one-off build: ISCC /DAppVersion=1.1 Calabrass.iss
#ifndef AppVersion
  #define AppVersion "1.0.3"
#endif
#define AppPublisher "Calabrass"
#define AppCopyright "Copyright (c) 2026 Mubasher Chaudhary"
#define AppExeName "Calabrass.exe"

; A stable identity for the product: it is what lets a newer build install over
; an older one instead of landing beside it, and what the entry in Apps &
; features is keyed on. It must never change.
#define AppId "{{1D7CF26E-0CB4-474A-975D-893EDDEEB779}"

[Setup]
AppId={#AppId}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppCopyright={#AppCopyright}
VersionInfoCompany={#AppPublisher}
VersionInfoDescription={#AppName} Setup
VersionInfoProductName={#AppName}
VersionInfoProductVersion={#AppVersion}
VersionInfoVersion=1.0.3.0
VersionInfoCopyright={#AppCopyright}

; Per-user by default: no administrator prompt, and the program lives with the
; rest of the user's software. The location page is left in, so it can still be
; pointed somewhere else. An administrator can ask for a machine-wide install
; from the command line with /ALLUSERS.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=commandline
DefaultDirName={localappdata}\Programs\{#AppName}
DefaultGroupName={#AppName}
AllowNoIcons=yes
DisableDirPage=no
DisableWelcomePage=no
DisableProgramGroupPage=no
WizardStyle=modern

OutputDir=out
OutputBaseFilename=Calabrass-setup-v{#AppVersion}-x64
Compression=lzma2/max
SolidCompression=yes
SetupIconFile=..\Backend\desktop\build\windows\icon.ico
UninstallDisplayIcon={app}\{#AppExeName}
UninstallDisplayName={#AppName}

; The app is one executable, so this only ever needs to run on a 64-bit PC.
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

; Windows will not let a running program be replaced. Setup asks the user to
; close it rather than failing at the last step.
CloseApplications=yes
RestartApplications=no

; Shown on the licence page, and installed beside the app.
LicenseFile=..\LICENSE

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
; Checked by default: a desktop shortcut is what most people expect.
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
; Not checked by default: the MCP servers hand an AI control of Excel and of the
; keyboard and windows, so they are opted into, never assumed.
Name: "mcp"; Description: "Include the MCP servers (Excel and Windows control), so Calabrass can read and write spreadsheets and drive the desktop"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "..\Backend\desktop\build\bin\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
; The background host. Calabrass.exe starts it when no host is running, so
; scheduled tasks keep running after the window closes; `calabrass-backend stop`
; ends it.
Source: "..\Backend\desktop\build\bin\calabrass-backend.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
; Only the built servers ship. Their source, and every server the installer was
; not asked to include, stays in the repository.
Source: "..\MCP\excel\excel-mcp.exe"; DestDir: "{app}\MCP\excel"; Flags: ignoreversion; Tasks: mcp
Source: "..\MCP\win-control\win-control-mcp.exe"; DestDir: "{app}\MCP\win-control"; Flags: ignoreversion; Tasks: mcp

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"
Name: "{group}\Stop Calabrass in the background"; Filename: "{app}\calabrass-backend.exe"; Parameters: "stop"; Comment: "Stops the Calabrass host. Scheduled tasks stop with it."
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(AppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; A host left running would hold its own files open and block their removal.
Filename: "{app}\calabrass-backend.exe"; Parameters: "stop"; Flags: runhidden; RunOnceId: "StopCalabrassHost"

[UninstallDelete]
; Setup removes the files it copied, but not a folder the servers filled in
; afterwards, so the MCP folder goes as a whole.
Type: filesandordirs; Name: "{app}\MCP"

[Code]
const
  // The WebView2 runtime registers itself under Edge's update client key, once
  // per machine and once per user. Only its presence matters, not its version.
  WebView2ClientId = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
  WebView2Url = 'https://go.microsoft.com/fwlink/p/?LinkId=2124703';

// The app's window is rendered by Microsoft's WebView2 runtime. Windows 10 and
// 11 almost always ship it, but a stripped-down or freshly imaged PC may not,
// and without it Calabrass would open and show nothing at all. Saying so now is
// kinder than a blank window later.
function HasWebView2: Boolean;
var
  Version: String;
begin
  Result :=
    RegQueryStringValue(HKLM, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\' + WebView2ClientId, 'pv', Version) or
    RegQueryStringValue(HKLM, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\' + WebView2ClientId, 'pv', Version) or
    RegQueryStringValue(HKCU, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\' + WebView2ClientId, 'pv', Version) or
    RegQueryStringValue(HKCU, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\' + WebView2ClientId, 'pv', Version);
end;

function InitializeSetup: Boolean;
var
  ErrorCode: Integer;
begin
  Result := True;
  if HasWebView2 then
    Exit;

  if MsgBox('Calabrass needs the Microsoft WebView2 runtime, which is not on this PC yet.' + #13#10 + #13#10 +
            'Installation can carry on, but Calabrass will not open a window until it is installed.' + #13#10 + #13#10 +
            'Open the download page in your browser now?',
            mbConfirmation, MB_YESNO) = IDYES then
    ShellExec('open', WebView2Url, '', '', SW_SHOWNORMAL, ewNoWait, ErrorCode);
end;

// Accounts are per person, and what is in the data folder belongs to the person
// who made it: chats, saved provider keys, workspaces. Removing the program is
// not consent to remove any of that, so it is asked for, and the safe answer is
// the one that is preselected.
function InitializeUninstall: Boolean;
var
  DataDir: String;
begin
  Result := True;
  if UninstallSilent then
    Exit;

  DataDir := ExpandConstant('{localappdata}\Calabrass');
  if not DirExists(DataDir) then
    Exit;

  if MsgBox('Also delete your Calabrass data?' + #13#10 + #13#10 +
            'That removes your chats, saved provider keys and workspaces from:' + #13#10 +
            DataDir + #13#10 + #13#10 +
            'Choose No to keep them, so installing Calabrass again picks up where you left off.',
            mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
    DelTree(DataDir, True, True, True);
end;

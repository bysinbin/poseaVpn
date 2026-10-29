; Inno Setup Script for PoseaVPN
#define MyAppName "PoseaVPN"
#define MyAppVersion "1.2.0"
#define MyAppPublisher "Posea"
#define MyAppURL "https://github.com/bysinbin/poseaVpn"
#define MyAppExeName "poseavpn-multi.exe"

[Setup]
AppId={{8B3B8B9B-79C0-4660-84E3-559B283E4C7A}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
OutputDir=.
OutputBaseFilename=PoseaVPN-Setup-x64
SetupIconFile=..\build\windows\icon.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "turkish"; MessagesFile: "compiler:Languages\Turkish.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "..\bin\poseavpn-multi.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\poseavpn-single.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\poseavpn-tray.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\poseavpn.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\poseavpn-service.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\wintun.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\openvpn\*"; DestDir: "{app}\openvpn"; Flags: ignoreversion recursesubdirs createallsubdirs

[Dirs]
Name: "{commonappdata}\PoseaVPN"
Name: "{commonappdata}\PoseaVPN\logs"; Permissions: users-modify
Name: "{commonappdata}\PoseaVPN\profiles"; Permissions: users-modify

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\PoseaVPN Tekli Tünel"; Filename: "{app}\poseavpn-single.exe"
Name: "{group}\PoseaVPN Sistem Tepsisi"; Filename: "{app}\poseavpn-tray.exe"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\poseavpn-service.exe"; Parameters: "--install"; Flags: runhidden
Filename: "sc.exe"; Parameters: "start PoseaVPN"; Flags: runhidden
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\poseavpn-service.exe"; Parameters: "--uninstall"; Flags: runhidden
Filename: "taskkill.exe"; Parameters: "/F /IM poseavpn-multi.exe /IM poseavpn-single.exe /IM poseavpn-tray.exe"; Flags: runhidden

[UninstallDelete]
Type: filesandordirs; Name: "{app}"

Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
## These !defines are evaluated before the !include below, so they win over the
## wails-generated fallbacks in wails_tools.nsh — which lets us hand-pick branding
## without editing the "DO NOT EDIT" generated file.
####
!define INFO_COMPANYNAME    "imonior"
!define INFO_PRODUCTNAME    "WireGuide Plus"
!define INFO_COPYRIGHT      "© 2026 imonior"
# UNINST_KEY_NAME drives the registry key under Uninstall\ and the "Programs and
# Features" display. Default is "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}" =
# "imoniorWireGuide Plus", which is awkward — override to the clean product name.
!define UNINST_KEY_NAME     "WireGuide Plus"
# Override the wails default ("wireguide") so every artifact name starts with
# the branded "wireguideplus" prefix — OutFile below becomes
# wireguideplus-<arch>-installer.exe. The built program itself is installed as
# PRODUCT_EXECUTABLE, which makensis receives via -DPRODUCT_EXECUTABLE=...
# (arch-suffixed), see build/windows/Taskfile.yml create:nsis:installer.
!define INFO_PROJECTNAME    "wireguideplus"
####
## !define INFO_PROJECTNAME    "my-project" # Default "wireguide"
## !define INFO_PRODUCTVERSION "1.1.0"     # Default "0.1.0"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
Page custom StartMenuPage StartMenuPageLeave # 快捷方式选项页（默认创建开始菜单，可取消）
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
# Single folder under Program Files — the wails default "<company>\<product>" nests
# "imonior\WireGuide Plus" which is ugly when company is just a github handle.
# The 32-bit build installs under $PROGRAMFILES (Program Files / Program Files (x86));
# 64-bit builds go to $PROGRAMFILES64.
!ifdef SUPPORTS_X86
InstallDir "$PROGRAMFILES\${INFO_PRODUCTNAME}"
!else
InstallDir "$PROGRAMFILES64\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show # This will always show the installation details.

Var CreateStartMenu # "1" = create Start Menu shortcuts (default), "0" = skip
Var StartMenuCheck  # nsDialogs checkbox handle on the shortcut options page

# Shortcut options page shown after the directory selection page. The Start
# Menu shortcut (with uninstall entry) is created by default but can be
# deselected; the desktop shortcut is always created and offers no choice.
Function StartMenuPage
    !insertmacro MUI_HEADER_TEXT "快捷方式选项" "选择要创建的快捷方式"
    nsDialogs::Create 1018
    Pop $0
    ${If} $0 == error
        Abort
    ${EndIf}

    ${NSD_CreateLabel} 0 0 100% 36u "将在开始菜单创建 WireGuide Plus 快捷方式（含“卸载 WireGuide Plus”入口）。桌面快捷方式始终创建，无需选择。"
    Pop $0
    ${NSD_CreateCheckBox} 0 44u 100% 24u "创建开始菜单快捷方式（含卸载入口）"
    Pop $StartMenuCheck
    ${NSD_SetState} $StartMenuCheck ${BST_CHECKED}

    nsDialogs::Show
FunctionEnd

Function StartMenuPageLeave
    ${NSD_GetState} $StartMenuCheck $0
    ${If} $0 == ${BST_CHECKED}
        StrCpy $CreateStartMenu "1"
    ${Else}
        StrCpy $CreateStartMenu "0"
    ${EndIf}
FunctionEnd

Function .onInit
    StrCpy $CreateStartMenu "1" # silent installs skip the page — keep the default
    !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    # The GUI tray app and the elevated helper are the same exe, and Windows
    # locks running images — without this, an in-place upgrade dies with
    # "Error opening file for writing: ${PRODUCT_EXECUTABLE}". The installer
    # runs elevated, so taskkill reaches the SYSTEM-level helper too. Exit code
    # is ignored: 128 just means nothing was running.
    nsExec::ExecToLog `taskkill /F /IM "${PRODUCT_EXECUTABLE}"`
    Pop $0
    Sleep 500

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # wireguard-go loads the architecture-specific wintun DLL (wintun-amd64.dll /
    # wintun-x86.dll / wintun-arm64.dll) from the EXE directory at first TUN
    # creation. Bundled here by `task vendor:wintun` during build; the filename
    # is picked from the arch flag makensis received for this installer.
    !ifdef ARG_WAILS_AMD64_BINARY
        File "..\..\..\bin\wintun-amd64.dll"
    !else ifdef ARG_WAILS_ARM64_BINARY
        File "..\..\..\bin\wintun-arm64.dll"
    !else
        File "..\..\..\bin\wintun-x86.dll"
    !endif

    # Desktop shortcut: always created — the user cannot opt out.
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    # Start Menu shortcuts (app + uninstall entry): created by default, can be
    # deselected on the shortcut options page. The uninstall entry reuses the
    # app icon (4th CreateShortCut arg) so its tray/icon matches the app.
    ${If} $CreateStartMenu == "1"
        CreateShortCut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
        CreateShortCut "$SMPROGRAMS\卸载 ${INFO_PRODUCTNAME}.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    ${EndIf}

    # Put the install dir on the system PATH so `wireguideplus ctl ...` is
    # callable from any terminal (the same binary is the GUI and the CLI).
    # Done via PowerShell/.NET rather than raw NSIS registry edits: .NET
    # reads and writes the full PATH value (NSIS's ReadRegStr truncates at
    # 1024 chars and would corrupt a long PATH), skips if already present,
    # and broadcasts WM_SETTINGCHANGE so new shells pick it up.
    #
    # $INSTDIR is passed to PowerShell as a PROCESS ENVIRONMENT VARIABLE,
    # never interpolated into the script source — a user-chosen install
    # path can legally contain a single quote, which would otherwise break
    # out of a PowerShell string literal and run as code under the
    # installer's admin token. `"` can't appear in a Windows path, so the
    # SetEnvironmentVariable marshalling below is safe. $$x is an escaped
    # literal `$x` for PowerShell.
    System::Call 'kernel32::SetEnvironmentVariable(t "WIREGUIDE_DIR", t "$INSTDIR")'
    nsExec::ExecToLog `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$d=$$env:WIREGUIDE_DIR; $$p=[Environment]::GetEnvironmentVariable('Path','Machine'); if(($$p -split ';') -notcontains $$d){[Environment]::SetEnvironmentVariable('Path',$$p.TrimEnd(';')+';'+$$d,'Machine')}"`
    Pop $0
    ${If} $0 != 0
        DetailPrint "Note: could not add WireGuide to PATH (exit $0). Use the full path to wireguide.exe, or add it to PATH manually."
    ${EndIf}

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    
    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # Same as install: both the tray app and the helper hold the exe image
    # open, so RMDir /r would silently leave ${PRODUCT_EXECUTABLE} behind.
    nsExec::ExecToLog `taskkill /F /IM "${PRODUCT_EXECUTABLE}"`
    Pop $0
    Sleep 500

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    # Remove the install dir from the system PATH (mirror of the install
    # step; $INSTDIR passed via env var, not interpolated — see install).
    System::Call 'kernel32::SetEnvironmentVariable(t "WIREGUIDE_DIR", t "$INSTDIR")'
    nsExec::ExecToLog `powershell -NoProfile -ExecutionPolicy Bypass -Command "$$d=$$env:WIREGUIDE_DIR; $$p=[Environment]::GetEnvironmentVariable('Path','Machine'); $$n=(($$p -split ';') | Where-Object { $$_ -ne $$d }) -join ';'; [Environment]::SetEnvironmentVariable('Path',$$n,'Machine')"`

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\卸载 ${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

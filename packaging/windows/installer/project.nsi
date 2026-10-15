Unicode true
!define REQUEST_EXECUTION_LEVEL "admin"
!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName" "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright" "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName" "${INFO_PRODUCTNAME}"
ManifestDPIAware true

!include "MUI.nsh"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext
    !insertmacro wails.webview2runtime
    nsExec::ExecToLog 'sc.exe stop ProxityEngine'
    Pop $0
    Sleep 4000

    SetOutPath $INSTDIR
    !insertmacro wails.files
    File "..\..\bin\proxity-service.exe"
    File "..\..\bin\WinDivert.dll"
    File "..\..\bin\WinDivert64.sys"
    DetailPrint "Installing ProxityEngine service"
    ExecWait '"$INSTDIR\proxity-service.exe" -install' $0
    ${If} $0 != 0
        DetailPrint "WARNING: service install returned $0"
        IfSilent +2 0
            MessageBox MB_ICONEXCLAMATION "Proxity was installed, but its background service could not start (code $0).$\r$\nUse the recovery button in Proxity or reinstall."
    ${EndIf}

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext
    ${If} ${FileExists} "$INSTDIR\proxity-service.exe"
        DetailPrint "Removing ProxityEngine service"
        ExecWait '"$INSTDIR\proxity-service.exe" -uninstall' $0
        ${If} $0 != 0
            MessageBox MB_ICONSTOP "Proxity could not remove its background service (code $0). Uninstall has stopped; retry after resolving the service error."
            Abort
        ${EndIf}
    ${EndIf}

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
    RMDir /r $INSTDIR
    SetShellVarContext all
    RMDir /r "$APPDATA\Proxity"
    !insertmacro wails.setShellContext
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    !insertmacro wails.deleteUninstaller
SectionEnd

@echo off
rem Run in an x64 Native Tools Command Prompt for VS 2022 or newer.
setlocal
cd /d "%~dp0" || exit /b 1
cl /nologo /utf-8 /std:c++17 /O2 /MT /EHsc /W4 /Brepro /DUNICODE /D_UNICODE /I . forever-launcher.cpp /Fe:"..\app\forever-connector.exe" /link /SUBSYSTEM:WINDOWS /MANIFEST:EMBED /Brepro shell32.lib ole32.lib user32.lib crypt32.lib ws2_32.lib advapi32.lib credui.lib bcrypt.lib winhttp.lib comctl32.lib gdi32.lib
exit /b %errorlevel%

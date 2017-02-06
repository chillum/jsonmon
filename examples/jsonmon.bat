@echo off
rem jsonmon start-up script for Windows

rem HTTP host & port
rem Set host to [::] to listen on all interfaces
set HOST=localhost
set PORT=3000

rem Include sh.exe to PATH
rem set PATH=C:\Program Files (x86)\Git\bin;%PATH%

rem Directory holding jsonmon.exe and config.yml
cd %SYSTEMDRIVE%\jsonmon

jsonmon.exe -syslog config.yml

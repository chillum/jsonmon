rem jsonmon start script for Windows
rem If you want to start jsonmon automatically, you can use Task Scheduler
@echo off

rem HTTP host & port
rem Set host to [::] to listen on all interfaces
set HOST=localhost
set PORT=3000

rem Include `sh` in PATH
rem set PATH=C:\Program Files (x86)\Git\bin;%PATH%

rem Directory holding jsonmon.exe and config.yml
cd %SYSTEMDRIVE%\jsonmon

jsonmon.exe -syslog config.yml

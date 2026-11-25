@echo off
cd /d "%~dp0"
python -m pip install --quiet --disable-pip-version-check bleak
title WS90 -^> Webcam Manager
echo Invio dei dati del WS90 a Webcam Manager ogni 60 secondi.
echo NON chiudere questa finestra (puoi ridurla a icona).
echo.
python ws90_bridge.py
pause

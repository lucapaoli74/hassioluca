@echo off
cd /d "%~dp0"
echo Installo/controllo la libreria Bluetooth (solo la prima volta serve internet)...
python -m pip install --quiet --disable-pip-version-check bleak
echo.
echo MODALITA PROVA: mostro i dati del WS90 ogni 60 secondi, senza inviarli.
echo Per fermare: chiudi questa finestra.
echo.
python ws90_bridge.py --prova
pause

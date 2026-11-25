"""Avvia ws90_bridge.py in MODALITA' PROVA (mostra i dati, non li invia).

Basta fare doppio clic su questo file. Deve stare nella stessa cartella di ws90_bridge.py.
"""
import os
import runpy
import sys

cartella = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, cartella)
sys.argv = [os.path.join(cartella, "ws90_bridge.py"), "--prova"]
print("MODALITA' PROVA: mostro i dati del WS90 ogni 60 secondi, senza inviarli.")
print("Per fermare: chiudi questa finestra.\n")
try:
    runpy.run_path(sys.argv[0], run_name="__main__")
except Exception as e:  # noqa: BLE001
    print(f"\nERRORE: {e}")
    input("Premi INVIO per chiudere...")

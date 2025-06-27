#!/bin/sh
set -e

echo "Waiting until rabbit is ready"
sleep 20


echo "Starting Script 1..."
/app/script1_app
echo "Script 1 finished."

sleep 2
# Run the second application.
echo "Starting Script 2..."
/app/script2_app
echo "Script 2 finished."
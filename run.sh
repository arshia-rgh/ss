#!/bin/sh
set -e

echo "Starting Script 1..."
/app/script1_app
echo "Script 1 finished."

# Run the second application.
echo "Starting Script 2..."
/app/script2_app
echo "Script 2 finished."
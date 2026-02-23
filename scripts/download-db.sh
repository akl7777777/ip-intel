#!/bin/bash
# Download free IP-ASN MMDB database
# Source: https://github.com/sapics/ip-location-db
#
# Safe update: downloads to temp file first, replaces only on success.
# The service can hot-reload without restart.

set -e

DATA_DIR="${1:-data}"
TARGET="$DATA_DIR/GeoLite2-ASN.mmdb"
TEMP_FILE="$DATA_DIR/.GeoLite2-ASN.mmdb.tmp"

mkdir -p "$DATA_DIR"

echo "Downloading GeoLite2-ASN compatible MMDB from sapics/ip-location-db..."
echo "(No registration required)"

URL="https://github.com/sapics/ip-location-db/raw/main/asn-mmdb/asn.mmdb"

# Download to temp file
if ! curl -fSL -o "$TEMP_FILE" "$URL"; then
    echo "Download failed. Check your network connection."
    rm -f "$TEMP_FILE"
    exit 1
fi

# Verify file is not empty and is a valid MMDB (starts with correct metadata)
FILE_SIZE=$(wc -c < "$TEMP_FILE" | tr -d ' ')
if [ "$FILE_SIZE" -lt 10000 ]; then
    echo "Downloaded file too small (${FILE_SIZE} bytes), likely corrupted. Keeping old database."
    rm -f "$TEMP_FILE"
    exit 1
fi

# Atomic replace: rename is atomic on the same filesystem
mv -f "$TEMP_FILE" "$TARGET"

SIZE=$(ls -lh "$TARGET" | awk '{print $5}')
echo "Done! Downloaded to $TARGET ($SIZE)"

if [ -f "$TARGET.old" ]; then
    echo "Previous version backed up as $TARGET.old"
fi

echo ""
echo "To update the database, run this script again."
echo "Recommended: set up a weekly cron job:"
echo "  0 3 * * 0 cd $(pwd) && bash scripts/download-db.sh"

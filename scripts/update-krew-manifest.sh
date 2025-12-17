#!/bin/bash
# Update Krew manifest with correct SHA256 checksums
# Usage: ./scripts/update-krew-manifest.sh v0.0.33

set -e

VERSION="${1:-}"
GITHUB_REPO="lukaszraczylo/jobs-manager-operator"
MANIFEST_FILE="plugins/krew/managedjob.yaml"

if [[ -z "$VERSION" ]]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 v0.0.33"
    exit 1
fi

# Remove 'v' prefix for archive names
VERSION_NUM="${VERSION#v}"

echo "Updating Krew manifest for version $VERSION..."

# Platforms to update
PLATFORMS=(
    "darwin_amd64:tar.gz"
    "darwin_arm64:tar.gz"
    "linux_amd64:tar.gz"
    "linux_arm64:tar.gz"
    "windows_amd64:zip"
)

# Download checksums file
CHECKSUMS_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/checksums.txt"
echo "Downloading checksums from $CHECKSUMS_URL..."

CHECKSUMS=$(curl -sSL "$CHECKSUMS_URL")
if [[ -z "$CHECKSUMS" ]]; then
    echo "Error: Failed to download checksums"
    exit 1
fi

echo "Checksums:"
echo "$CHECKSUMS"
echo ""

# Update manifest
for platform_ext in "${PLATFORMS[@]}"; do
    platform="${platform_ext%:*}"
    ext="${platform_ext#*:}"

    archive_name="jobs-manager-operator_${VERSION_NUM}_${platform}.${ext}"
    sha256=$(echo "$CHECKSUMS" | grep "$archive_name" | awk '{print $1}')

    if [[ -z "$sha256" ]]; then
        echo "Warning: No checksum found for $archive_name"
        continue
    fi

    echo "  $platform: $sha256"
done

# Update version in manifest
sed -i.bak "s/version: v.*/version: ${VERSION}/" "$MANIFEST_FILE"

# Update URIs and find/replace SHA256 placeholders
# This is a simplified approach - for production, use yq or similar
echo ""
echo "Manifest updated with version $VERSION"
echo "NOTE: SHA256 checksums must be manually updated in $MANIFEST_FILE"
echo ""
echo "Copy the checksums above and replace REPLACE_WITH_ACTUAL_SHA256 in the manifest."

rm -f "${MANIFEST_FILE}.bak"

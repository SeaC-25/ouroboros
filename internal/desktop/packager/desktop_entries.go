package packager

import (
	"fmt"
	"strings"
)

// LinuxDesktopEntryName is the XDG desktop entry file name, matching
// gui/scripts/package-desktop.mjs.
const LinuxDesktopEntryName = "dev.ouroboros.Ouroboros.desktop"

// PackagedIconRelativePath is the icon path relative to a packaged/installed
// app directory.
const PackagedIconRelativePath = "resources/app/dist/main/assets/ouroboros-icon.png"

// linuxPackageDesktopEntry is the relocatable desktop entry shipped inside
// the package directory (1:1 with the mjs template).
func linuxPackageDesktopEntry() string {
	return fmt.Sprintf(`[Desktop Entry]
Version=1.0
Type=Application
Name=Ouroboros
GenericName=AI Security IDE
GenericName[zh_CN]=AI 安全 IDE
Comment=Ouroboros AI Security IDE
Comment[zh_CN]=Ouroboros AI 安全 IDE
Icon=dev.ouroboros.Ouroboros
Exec=%s %%k %%U
Terminal=false
StartupNotify=false
StartupWMClass=Ouroboros
Categories=Development;IDE;
Keywords=AI;IDE;security;agent;
MimeType=x-scheme-handler/ouroboros;
`, relocatablePackageDesktopExecValue())
}

// LinuxDesktopEntryFor renders an installed desktop entry pointing into
// packageDir. Exported so the installer can reuse it.
func LinuxDesktopEntryFor(join func(elem ...string) string, packageDir string) string {
	appBinary := join(packageDir, "Ouroboros")
	return fmt.Sprintf(`[Desktop Entry]
Version=1.0
Type=Application
Name=Ouroboros
GenericName=AI Security IDE
GenericName[zh_CN]=AI 安全 IDE
Comment=Ouroboros AI Security IDE
Comment[zh_CN]=Ouroboros AI 安全 IDE
Icon=%s
Exec=%s %%U
Path=%s
Terminal=false
StartupNotify=false
StartupWMClass=Ouroboros
Categories=Development;IDE;
Keywords=AI;IDE;security;agent;
MimeType=x-scheme-handler/ouroboros;
`, desktopEntryValue(join(packageDir, PackagedIconRelativePath)),
		desktopExecValue(appBinary),
		desktopEntryValue(packageDir))
}

// LinuxDesktopInstallerScript is the install-desktop-entry.sh bundled into
// the package directory.
func LinuxDesktopInstallerScript(join func(elem ...string) string) string {
	entry := LinuxDesktopEntryFor(join, "${PACKAGE_DIR}")
	return fmt.Sprintf(`#!/bin/sh
set -eu

PACKAGE_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
case "${XDG_DATA_HOME:-}" in
  /*) DATA_HOME="$XDG_DATA_HOME" ;;
  *) DATA_HOME="$HOME/.local/share" ;;
esac
APPLICATIONS_DIR="$DATA_HOME/applications"
DESKTOP_FILE="$APPLICATIONS_DIR/%s"

mkdir -p "$APPLICATIONS_DIR"
if [ -e "$DESKTOP_FILE" ] || [ -L "$DESKTOP_FILE" ]; then
  rm -f -- "$DESKTOP_FILE"
fi
cat > "$DESKTOP_FILE" <<EOF
%sEOF
chmod 0644 "$DESKTOP_FILE"
update-desktop-database "$APPLICATIONS_DIR" >/dev/null 2>&1 || true
printf 'Ouroboros desktop entry installed: %%s\n' "$DESKTOP_FILE"
`, LinuxDesktopEntryName, entry)
}

func relocatablePackageDesktopExecValue() string {
	return `/bin/sh -c "desktop_file=\$1; case \$desktop_file in file://*) desktop_file=\${desktop_file#file://};; esac; package_dir=\$(dirname -- \"\$desktop_file\"); exec \"\$package_dir/Ouroboros\" \"\$@\"" ouroboros-package`
}

func desktopExecValue(value string) string {
	r := strings.NewReplacer(`"`, `\"`, `\`, `\\`)
	return `"` + r.Replace(value) + `"`
}

func desktopEntryValue(value string) string {
	return strings.ReplaceAll(value, `\`, `\\`)
}

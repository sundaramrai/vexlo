#!/bin/sh
set -eu

case "$(uname -s)" in
  Linux) platform=linux ;;
  Darwin) platform=darwin ;;
  *) echo 'Installer test requires Linux or macOS.' >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo 'Unsupported test architecture.' >&2; exit 1 ;;
esac

temp_base="${TMPDIR:-/tmp}"
temp_base="${temp_base%/}"
test_root="$(mktemp -d "${temp_base}/vexlo-installer-test.XXXXXX")"
case "$test_root" in
  "${temp_base}"/vexlo-installer-test.*) ;;
  *) echo 'Unexpected temporary directory.' >&2; exit 1 ;;
esac
trap 'rm -rf -- "$test_root"' EXIT HUP INT TERM

fixture="$test_root/release"
mkdir -p "$fixture"
binary="vexlo-${platform}-${arch}"
archive="${binary}.tar.gz"
printf '#!/bin/sh\nprintf "fixture cli\\n"\n' > "$fixture/$binary"
chmod 755 "$fixture/$binary"
tar -czf "$fixture/$archive" -C "$fixture" "$binary"
if command -v sha256sum >/dev/null 2>&1; then
  checksum="$(sha256sum "$fixture/$archive" | awk '{print $1}')"
else
  checksum="$(shasum -a 256 "$fixture/$archive" | awk '{print $1}')"
fi
printf '%s  %s\n' "$checksum" "$archive" > "$fixture/SHA256SUMS.txt"

# The test script differs only in the download origin; no real release or
# developer installation directory is touched.
sed "s|https://github.com/sundaramrai/vexlo/releases/latest/download|file://${fixture}|" \
  scripts/install.sh > "$test_root/install.sh"

home_on_path="$test_root/home-on-path"
mkdir -p "$home_on_path"
HOME="$home_on_path" PATH="$home_on_path/.local/bin:/usr/bin:/bin" \
  sh "$test_root/install.sh"
test "$("$home_on_path/.local/bin/vexlo")" = 'fixture cli'
test ! -e "$home_on_path/.profile"

home_new_path="$test_root/home-new-path"
mkdir -p "$home_new_path"
HOME="$home_new_path" SHELL=/bin/bash PATH=/usr/bin:/bin \
  sh "$test_root/install.sh"
test "$("$home_new_path/.local/bin/vexlo")" = 'fixture cli'
test "$(grep -Fxc 'export PATH="$HOME/.local/bin:$PATH"' "$home_new_path/.profile")" = 1
test "$(grep -Fxc 'export PATH="$HOME/.local/bin:$PATH"' "$home_new_path/.bashrc")" = 1
HOME="$home_new_path" SHELL=/bin/bash PATH=/usr/bin:/bin \
  sh "$test_root/install.sh"
test "$(grep -Fxc 'export PATH="$HOME/.local/bin:$PATH"' "$home_new_path/.profile")" = 1

printf '%064d  %s\n' 0 "$archive" > "$fixture/SHA256SUMS.txt"
home_bad_checksum="$test_root/home-bad-checksum"
mkdir -p "$home_bad_checksum"
if HOME="$home_bad_checksum" PATH="$home_bad_checksum/.local/bin:/usr/bin:/bin" \
  sh "$test_root/install.sh" > "$test_root/bad-checksum.log" 2>&1; then
  echo 'Installer accepted a bad checksum.' >&2
  exit 1
fi
test ! -e "$home_bad_checksum/.local/bin/vexlo"

echo 'Unix installer tests passed.'

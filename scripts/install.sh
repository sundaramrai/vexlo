#!/bin/sh
set -eu

case "$(uname -s)" in
  Linux) platform=linux ;;
  Darwin) platform=darwin ;;
  *) echo 'Unsupported OS. Use a Windows PowerShell installer or a release archive.' >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo 'Unsupported CPU architecture.' >&2; exit 1 ;;
esac

command -v curl >/dev/null 2>&1 || { echo 'curl is required.' >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo 'tar is required.' >&2; exit 1; }
command -v install >/dev/null 2>&1 || { echo 'install is required.' >&2; exit 1; }

archive="vexlo-${platform}-${arch}.tar.gz"
release="https://github.com/sundaramrai/vexlo/releases/latest/download"
work_dir="$(mktemp -d)"
trap 'rm -rf -- "$work_dir"' EXIT HUP INT TERM

curl -fsSL "${release}/${archive}" -o "${work_dir}/${archive}"
curl -fsSL "${release}/SHA256SUMS.txt" -o "${work_dir}/SHA256SUMS.txt"
expected="$(awk -v name="$archive" '$2 == name {print $1}' "${work_dir}/SHA256SUMS.txt")"
if [ -z "$expected" ]; then
  echo "Release checksum for ${archive} is missing." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${work_dir}/${archive}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "${work_dir}/${archive}" | awk '{print $1}')"
else
  echo 'A SHA-256 checksum tool is required.' >&2
  exit 1
fi
if [ "$expected" != "$actual" ]; then
  echo 'Release checksum does not match.' >&2
  exit 1
fi

binary="vexlo-${platform}-${arch}"
entries="$(tar -tzf "${work_dir}/${archive}")"
if [ "$entries" != "$binary" ]; then
  echo 'Release archive has unexpected contents.' >&2
  exit 1
fi
tar -xzf "${work_dir}/${archive}" -C "$work_dir"

path_has() {
  case ":${PATH}:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

user_bin="${HOME}/.local/bin"
if path_has "$user_bin"; then
  mkdir -p "$user_bin"
  install -m 0755 "${work_dir}/${binary}" "${user_bin}/vexlo"
  destination="$user_bin"
elif path_has "${HOME}/bin"; then
  mkdir -p "${HOME}/bin"
  install -m 0755 "${work_dir}/${binary}" "${HOME}/bin/vexlo"
  destination="${HOME}/bin"
elif path_has /usr/local/bin && [ -d /usr/local/bin ]; then
  if [ -w /usr/local/bin ]; then
    install -m 0755 "${work_dir}/${binary}" /usr/local/bin/vexlo
    destination=/usr/local/bin
  fi
fi

if [ -z "${destination:-}" ]; then
  mkdir -p "$user_bin"
  install -m 0755 "${work_dir}/${binary}" "${user_bin}/vexlo"
  destination="$user_bin"
  # A piped installer cannot change its parent shell's PATH. Set up future
  # terminals automatically instead of asking the developer to edit it.
  path_line='export PATH="$HOME/.local/bin:$PATH"'
  add_path_to_profile() {
    profile=$1
    if [ ! -f "$profile" ] || ! grep -Fqx "$path_line" "$profile"; then
      printf '\n%s\n' "$path_line" >> "$profile"
    fi
  }
  add_path_to_profile "${HOME}/.profile"
  case "${SHELL:-}" in
    */bash) add_path_to_profile "${HOME}/.bashrc" ;;
    */zsh) add_path_to_profile "${HOME}/.zshrc" ;;
    */fish)
      mkdir -p "${HOME}/.config/fish"
      fish_profile="${HOME}/.config/fish/config.fish"
      fish_line='set -gx PATH $HOME/.local/bin $PATH'
      if [ ! -f "$fish_profile" ] || ! grep -Fqx "$fish_line" "$fish_profile"; then
        printf '\n%s\n' "$fish_line" >> "$fish_profile"
      fi
      ;;
  esac
  printf 'Installed %s. Open a new terminal, then run: vexlo http 3000\n' "${destination}/vexlo"
else
  printf 'Installed %s. Run: vexlo http 3000\n' "${destination}/vexlo"
fi

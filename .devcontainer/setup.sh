#!/bin/bash
set -e

curl https://mise.run | sh

~/.local/bin/mise trust
~/.local/bin/mise install

cat >> ~/.bashrc << 'EOF'
export PATH="$HOME/.local/bin:$PATH"
eval "$(~/.local/bin/mise activate bash)"
EOF
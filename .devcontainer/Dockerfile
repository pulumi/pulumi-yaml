FROM mcr.microsoft.com/devcontainers/base:ubuntu

USER vscode

RUN curl -fsSL https://mise.run | sh
RUN echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
RUN echo 'eval "$(mise activate bash)"' >> ~/.bashrc

# If you run the devcontainer within an editor, the editor will start trying to
# fire up the LSP _before_ the `postCreateCommand` hook runs. However, we need
# to run `mise install` in the `postCreateCommand` as it's the first time we
# have access to the repository itself. So, this ensures that a Go binary
# exists to start up the Go LSP in the first place, and subsequent mentions of
# Go within the container will refer to the local `.mise.toml` version, which
# will take priority in the `$PATH` list.
RUN ~/.local/bin/mise use -g go@latest

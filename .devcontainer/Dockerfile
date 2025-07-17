FROM mcr.microsoft.com/devcontainers/base:ubuntu

USER vscode

RUN curl -fsSL https://mise.run | sh
RUN echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
RUN echo 'eval "$(mise activate bash)"' >> ~/.bashrc
RUN ~/.local/bin/mise use -g go@latest

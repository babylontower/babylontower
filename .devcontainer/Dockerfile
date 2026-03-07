# Base image: Go 1.25 on Debian Trixie
FROM mcr.microsoft.com/devcontainers/go:2-1.25-trixie

# Switch to root to install packages
USER root

# Update package lists and install all required packages
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get install -y --no-install-recommends \
    # Git LFS and Git
    git-lfs \
    git \
    # Node.js (installed via apt, will be updated by devcontainer feature) \
    nodejs \
    npm \
    # Protobuf and build tools
    protobuf-compiler \
    pkg-config \
    # Wayland dependencies
    libwayland-dev \
    libxkbcommon-dev \
    libxkbcommon0 \
    wayland-protocols \
    # X11 dependencies
    libx11-dev \
    libx11-xcb-dev \
    libxcursor-dev \
    libxfixes-dev \
    libxrandr-dev \
    libxi-dev \
    libxinerama-dev \
    libxkbcommon-x11-dev \
    # EGL/GLES/Mesa dependencies
    libegl1-mesa-dev \
    libgles2-mesa-dev \
    libegl-dev \
    libgles2-mesa-dev \
    mesa-common-dev \
    # Vulkan dependencies
    libvulkan1 \
    libvulkan-dev \
    vulkan-tools \
    # GTK and UI dependencies
    libgtk-3-dev \
    libgtksourceview-4-dev \
    libayatana-appindicator3-dev \
    # SDL2
    libsdl2-dev \
    # AV/FFmpeg dependencies
    libavutil-dev \
    libavformat-dev \
    libavfilter-dev \
    libavdevice-dev \
    libswresample-dev \
    libswscale-dev \
    # Audio dependencies
    libasound2-dev \
    libpulse-dev \
    libjack-dev \
    # System dependencies
    libdbus-1-dev \
    libudev-dev \
    # Avahi for mDNS
    avahi-daemon \
    libnss-mdns \
    # Cross-compilation for Windows
    mingw-w64 \
    # Cleanup
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Initialize git-lfs
RUN git lfs install

# Set environment variables for pkg-config
ENV PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig:${PKG_CONFIG_PATH}

# Switch back to vscode user
USER vscode

# Default shell
SHELL ["/bin/bash", "-c"]

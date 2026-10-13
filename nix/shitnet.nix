{ pkgs }:
pkgs.stdenv.mkDerivation {
  pname = "shitnet";
  version = "0.1.0";
  src = pkgs.fetchurl {
    name = "shitnet-f5fdf6cd.tar.gz";
    url = "https://codeload.github.com/tethux/shitnet/tar.gz/f5fdf6cdbea52e6af9f5e596042c8e7cf91436e2";
    hash = "sha256-dZMBa1yUqolKcdkzXHv4wgPvrOUC5sMz4wzPsK+wi+M=";
  };
  nativeBuildInputs = [ pkgs.xmake pkgs.clang pkgs.clang-tools ];
  configurePhase = ''
    runHook preConfigure
    export XMAKE_ROOT=y
    export XMAKE_CONFIGDIR="$TMPDIR/xmake"
    # The module scanner invokes Clang directly and needs its wrapper's include flags.
    task_cxx_flags="$(cat ${pkgs.clang}/nix-support/libcxx-cxxflags ${pkgs.clang}/nix-support/libc-cflags ${pkgs.clang}/nix-support/cc-cflags | sed 's/-cxx-isystem /-isystem/g; s/-idirafter /-idirafter/g')"
    xmake f -m release --cxxflags="$task_cxx_flags" -y
    runHook postConfigure
  '';
  buildPhase = ''
    runHook preBuild
    xmake build -j "$NIX_BUILD_CORES" -y shitnet
    runHook postBuild
  '';
  installPhase = ''
    runHook preInstall
    xmake install -o "$out" shitnet
    runHook postInstall
  '';
}

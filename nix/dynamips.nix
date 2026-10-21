{ pkgs }:
pkgs.stdenvNoCC.mkDerivation {
  pname = "dynamips-bindings";
  version = "0.2.28";
  src = pkgs.fetchurl {
    url = "https://github.com/tethux/dynamips-downgrade/releases/download/v0.2.28/dynamips-downgrade_0.2.28_linux_amd64.tar.gz";
    hash = "sha256-t35uLjoa8nlfjBTP2MDEDkcbfgKZYMarJjWlgeH95bg=";
  };
  sourceRoot = ".";
  propagatedBuildInputs = [ pkgs.libelf pkgs.libpcap pkgs.libnsl pkgs.zlib pkgs.stdenv.cc.cc.lib ];
  dontStrip = true;
  installPhase = ''
    runHook preInstall
    mkdir -p "$out"
    cp -r include lib "$out/"
    runHook postInstall
  '';
}

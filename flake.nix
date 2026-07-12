{
  description = "tethux development and bare-metal CI canary hosts";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      disko,
      ...
    }:
    let
      system = "x86_64-linux";
      nixosSystem =
        hostModule:
        nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = {
            inherit self disko;
          };
          modules = [
            ./nix/modules/base.nix
            ./nix/modules/ci-canary-user.nix
            ./nix/modules/containers.nix
            ./nix/modules/fixture-registry.nix
            ./nix/modules/networking-lab.nix
            ./nix/modules/virtualization.nix
            ./nix/modules/tethux-test-host.nix
            hostModule
          ];
        };
      nixosInstallSystem =
        hostModule:
        nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = {
            inherit self disko;
          };
          modules = [
            disko.nixosModules.disko
            ./nix/installers/disko-laptop.nix
            ./nix/modules/base.nix
            ./nix/modules/ci-canary-user.nix
            ./nix/modules/containers.nix
            ./nix/modules/fixture-registry.nix
            ./nix/modules/networking-lab.nix
            ./nix/modules/virtualization.nix
            ./nix/modules/tethux-test-host.nix
            hostModule
          ];
        };
    in
    {
      nixosConfigurations = {
        canary-10-0-0-100 = nixosSystem ./nix/hosts/canary-10-0-0-100.nix;
        canary-former-10-0-0-12 = nixosSystem ./nix/hosts/canary-former-10-0-0-12.nix;
        canary-10-0-0-100-install = nixosInstallSystem ./nix/hosts/canary-10-0-0-100.nix;
        canary-former-10-0-0-12-install = nixosInstallSystem ./nix/hosts/canary-former-10-0-0-12.nix;
      };
    }
    // flake-utils.lib.eachSystem [ system ] (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
        goInputs = {
          src = ./.;
          vendorHash = "sha256-zo8OVMOf3TC5lbZfsE2wSi5N8xa+tnMcOuAg40DjFrI=";
          nativeBuildInputs = [ pkgs.pkg-config ];
          buildInputs = [ pkgs.libpcap ];
        };
        tethux = pkgs.buildGoModule (goInputs // {
          pname = "tethux";
          version = "0.0.0";
          subPackages = [ "cmd/tethux" ];
        });
      in
      {
        packages = {
          inherit tethux;
          default = tethux;
        };

        devShells =
          let
            cgoPcapEnv = {
              CGO_ENABLED = "1";
              CGO_CFLAGS = "-I${pkgs.libpcap}/include";
              CGO_LDFLAGS = "-L${pkgs.libpcap.lib}/lib -lpcap";
              LD_LIBRARY_PATH = "${pkgs.libpcap.lib}/lib";
            };
          in
          {
            ci = pkgs.mkShell (
              cgoPcapEnv
              // {
                packages = with pkgs; [
                  bashInteractive
                  git
                  go
                  golangci-lint
                  jq
                  libpcap
                  openssh
                  pkg-config
                  util-linux
                  zstd
                ];
              }
            );

            integration = pkgs.mkShell (
              cgoPcapEnv
              // {
                packages = with pkgs; [
                  bashInteractive
                  bridge-utils
                  containerd
                  curl
                  docker-client
                  git
                  go
                  golangci-lint
                  iproute2
                  jq
                  libpcap
                  openssh
                  pkg-config
                  podman
                  procps
                  runc
                  socat
                  tcpdump
                ];
              }
            );

            default = pkgs.mkShell (
              cgoPcapEnv
              // {
                packages = with pkgs; [
                  bashInteractive
                  bridge-utils
                  curl
                  dynamips
                  git
                  go
                  golangci-lint
                  gofumpt
                  gotools
                  iproute2
                  jq
                  libpcap
                  mise
                  nmap
                  pkg-config
                  qemu_kvm
                  socat
                  tcpdump
                ];

                shellHook = ''
                  export DOCKER_HOST="''${DOCKER_HOST:-unix:///var/run/docker.sock}"
                  export CONTAINER_HOST="''${CONTAINER_HOST:-unix:///run/podman/podman.sock}"
                  export CONTAINERD_ADDRESS="''${CONTAINERD_ADDRESS:-/run/containerd/containerd.sock}"
                '';
              }
            );
          };

        checks = {
          unit = pkgs.buildGoModule (goInputs // {
            pname = "tethux-unit-tests";
            version = "0.0.0";
            buildPhase = ''
              runHook preBuild
              runHook postBuild
            '';
            checkPhase = ''
              runHook preCheck
              go test ./...
              runHook postCheck
            '';
            installPhase = ''
              runHook preInstall
              touch "$out"
              runHook postInstall
            '';
          });

          build = tethux;
        };
      }
    );
}

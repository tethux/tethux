{ pkgs, ... }:

let
  registry = "127.0.0.1:5000";
  skopeoRegistries = pkgs.writeText "tethux-skopeo-registries.conf" ''
    unqualified-search-registries = []
  '';
  fixtureRoot = pkgs.buildEnv {
    name = "tethux-fixture-root";
    paths = [
      pkgs.busybox
      pkgs.dockerTools.fakeNss
    ];
    pathsToLink = [
      "/bin"
      "/etc"
      "/sbin"
    ];
  };
  mkFixture = name: marker: pkgs.dockerTools.buildLayeredImage {
    name = "tethux/${name}";
    tag = "1";
    contents = [ fixtureRoot ];
    config = {
      Cmd = [
        "sh"
        "-c"
        "echo ${marker}"
      ];
      Env = [ "PATH=/bin:/sbin" ];
      Labels = {
        "io.tethux.fixture" = marker;
        "org.opencontainers.image.source" = "nix/modules/fixture-registry.nix";
      };
    };
  };
  fixtureA = mkFixture "fixture-a" "fixture-a";
  fixtureB = mkFixture "fixture-b" "fixture-b";
in
{
  services.dockerRegistry = {
    enable = true;
    listenAddress = "127.0.0.1";
    port = 5000;
  };

  systemd.services.tethux-fixture-registry-seed = {
    description = "Seed the local tethux CI fixture registry";
    wantedBy = [ "multi-user.target" ];
    requires = [ "docker-registry.service" ];
    after = [ "docker-registry.service" ];
    restartTriggers = [
      fixtureA
      fixtureB
    ];
    path = [
      pkgs.coreutils
      pkgs.curl
      pkgs.skopeo
    ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      for attempt in $(seq 1 30); do
        if curl --fail --silent http://${registry}/v2/ >/dev/null; then
          break
        fi
        if [ "$attempt" -eq 30 ]; then
          echo "fixture registry did not become ready" >&2
          exit 1
        fi
        sleep 1
      done

      skopeo --registries-conf=${skopeoRegistries} copy --dest-tls-verify=false \
        docker-archive:${fixtureA} \
        docker://${registry}/tethux/fixture-a:1
      skopeo --registries-conf=${skopeoRegistries} copy --dest-tls-verify=false \
        docker-archive:${fixtureB} \
        docker://${registry}/tethux/fixture-b:1
    '';
  };

  environment.variables = {
    TETHUX_FIXTURE_IMAGE_A = "${registry}/tethux/fixture-a:1";
    TETHUX_FIXTURE_IMAGE_B = "${registry}/tethux/fixture-b:1";
    TETHUX_TEST_IMAGES = "${registry}/tethux/fixture-a:1,${registry}/tethux/fixture-b:1";
  };
}

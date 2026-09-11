{
  description = "Partner Watch desktop client";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.home-manager = {
    url = "github:nix-community/home-manager";
    inputs.nixpkgs.follows = "nixpkgs";
  };
  outputs =
    {
      self,
      nixpkgs,
      home-manager,
      ...
    }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      package = pkgs.buildGoModule {
        pname = "partner-watch-desktop";
        version = "0.1.0";
        src = ./.;
        vendorHash = "sha256-O/4ABKz1aDsPV2usCOq45JIGJ7DkGSyu0hn4cPqzRhQ=";
        subPackages = [ "cmd/partner-watch-desktop" ];
        nativeBuildInputs = [ pkgs.makeWrapper ];
        postFixup = ''
          wrapProgram $out/bin/partner-watch-desktop \
            --prefix PATH : ${
              pkgs.lib.makeBinPath [
                pkgs.grim
                pkgs.niri
              ]
            }
        '';
        meta = {
          description = "Partner Watch client for NixOS desktops";
          mainProgram = "partner-watch-desktop";
          platforms = [ "x86_64-linux" ];
        };
      };
      nixosEvaluation = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          self.nixosModules.default
          {
            system.stateVersion = "26.05";
            services.partner-watch-desktop = {
              enable = true;
              user = "test-user";
              serverUrl = "https://partner-watch.example.com";
              deviceName = "Test PC";
              acceptCaptures = true;
              forwardNotifications = true;
            };
          }
        ];
      };
      homeEvaluation = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [
          self.homeManagerModules.default
          {
            home = {
              username = "test-user";
              homeDirectory = "/home/test-user";
              stateVersion = "26.05";
            };
            services.partner-watch-desktop = {
              enable = true;
              serverUrl = "https://partner-watch.example.com";
              deviceName = "Test PC";
              acceptCaptures = true;
              forwardNotifications = true;
            };
          }
        ];
      };
    in
    {
      packages.${system}.default = package;
      apps.${system}.default = {
        type = "app";
        program = "${package}/bin/partner-watch-desktop";
        meta.description = "Run the Partner Watch desktop client";
      };
      formatter.${system} = pkgs.nixfmt-tree;
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          go_1_26
          gotools
          grim
          niri
          libnotify
        ];
      };
      homeManagerModules.default = import ./nix/home-manager-module.nix {
        partnerWatchPackage = self.packages.${system}.default;
      };
      nixosModules.default = import ./nix/nixos-module.nix {
        partnerWatchPackage = self.packages.${system}.default;
      };
      checks.${system} = {
        package = package;
        nixos-module = pkgs.writeText "partner-watch-nixos-module-check" (
          nixosEvaluation.config.systemd.user.services.partner-watch-desktop.serviceConfig.ExecStart
        );
        home-manager-module = pkgs.writeText "partner-watch-home-manager-module-check" (
          builtins.concatStringsSep " " homeEvaluation.config.systemd.user.services.partner-watch-desktop.Service.ExecStart
        );
      };
    };
}

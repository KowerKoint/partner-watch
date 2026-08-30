{
  description = "Partner Watch Android development environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
        config = {
          allowUnfree = true;
          android_sdk.accept_license = true;
        };
      };
      androidComposition = pkgs.androidenv.composeAndroidPackages {
        platformVersions = [ "36" "37" ];
        buildToolsVersions = [ "36.0.0" ];
        includeEmulator = false;
        includeSystemImages = false;
      };
      androidSdkRoot = "${androidComposition.androidsdk}/libexec/android-sdk";
      androidStudioLauncher = pkgs.writeShellScript "partner-watch-android-studio" ''
        export ANDROID_HOME="${androidSdkRoot}"
        export ANDROID_SDK_ROOT="${androidSdkRoot}"
        exec "${pkgs.android-studio}/bin/android-studio" "$@"
      '';
    in {
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          jdk21
          gradle
          androidComposition.androidsdk
          android-tools
        ];

        ANDROID_HOME = androidSdkRoot;
        ANDROID_SDK_ROOT = androidSdkRoot;
      };

      apps.${system}.android-studio = {
        type = "app";
        program = "${androidStudioLauncher}";
      };
    };
}

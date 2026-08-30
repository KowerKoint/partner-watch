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
    in {
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          jdk21
          gradle
          androidComposition.androidsdk
          android-tools
        ];

        ANDROID_HOME = "${androidComposition.androidsdk}/libexec/android-sdk";
        ANDROID_SDK_ROOT = "${androidComposition.androidsdk}/libexec/android-sdk";
      };

      apps.${system}.android-studio = {
        type = "app";
        program = "${pkgs.android-studio}/bin/android-studio";
      };
    };
}

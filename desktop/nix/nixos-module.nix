{ partnerWatchPackage }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.partner-watch-desktop;
  toml = pkgs.formats.toml { };
  configFile = toml.generate "partner-watch-config.toml" {
    server_url = cfg.serverUrl;
    device_name = cfg.deviceName;
    accept_captures = cfg.acceptCaptures;
    forward_notifications = cfg.forwardNotifications;
  };
  enroll = pkgs.writeShellScriptBin "partner-watch-desktop-enroll" ''
    exec ${cfg.package}/bin/partner-watch-desktop enroll --config ${configFile} --state "$HOME/.local/state/partner-watch/state.json" "$@"
  '';
in
{
  options.services.partner-watch-desktop = {
    enable = lib.mkEnableOption "Partner Watch desktop client";
    package = lib.mkOption {
      type = lib.types.package;
      default = partnerWatchPackage;
      description = "Partner Watch desktop client package.";
    };
    user = lib.mkOption {
      type = lib.types.str;
      description = "Desktop user for whom the user service is enabled.";
    };
    serverUrl = lib.mkOption {
      type = lib.types.str;
      example = "https://partner-watch.example.com";
      description = "Public HTTPS origin of the Partner Watch server.";
    };
    deviceName = lib.mkOption {
      type = lib.types.str;
      default = "NixOS PC";
      description = "Name shown for this desktop device.";
    };
    acceptCaptures = lib.mkEnableOption "screenshots requested by the partner";
    forwardNotifications = lib.mkEnableOption "desktop notifications to the partner";
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [
      cfg.package
      enroll
    ];
    systemd.user.services.partner-watch-desktop = {
      description = "Partner Watch desktop client";
      wantedBy = [ "graphical-session.target" ];
      partOf = [ "graphical-session.target" ];
      after = [
        "graphical-session.target"
        "network-online.target"
      ];
      wants = [ "network-online.target" ];
      unitConfig.ConditionUser = cfg.user;
      unitConfig.ConditionPathExists = "%h/.local/state/partner-watch/state.json";
      serviceConfig = {
        ExecStart = "${cfg.package}/bin/partner-watch-desktop run --config ${configFile} --state %h/.local/state/partner-watch/state.json";
        Restart = "on-failure";
        RestartSec = 5;
      };
    };
  };
}

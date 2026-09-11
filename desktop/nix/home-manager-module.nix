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
in
{
  options.services.partner-watch-desktop = {
    enable = lib.mkEnableOption "Partner Watch desktop client";
    package = lib.mkOption {
      type = lib.types.package;
      default = partnerWatchPackage;
      description = "Partner Watch desktop client package.";
    };
    serverUrl = lib.mkOption {
      type = lib.types.str;
      example = "https://partner-watch.example.com";
      description = "Public HTTPS origin of the Partner Watch server.";
    };
    deviceName = lib.mkOption {
      type = lib.types.str;
      default = config.home.username + " PC";
      description = "Name shown for this desktop device.";
    };
    acceptCaptures = lib.mkEnableOption "screenshots requested by the partner";
    forwardNotifications = lib.mkEnableOption "desktop notifications to the partner";
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];
    xdg.configFile."partner-watch/config.toml".source = toml.generate "partner-watch-config.toml" {
      server_url = cfg.serverUrl;
      device_name = cfg.deviceName;
      accept_captures = cfg.acceptCaptures;
      forward_notifications = cfg.forwardNotifications;
    };
    systemd.user.services.partner-watch-desktop = {
      Unit = {
        Description = "Partner Watch desktop client";
        After = [
          "graphical-session.target"
          "network-online.target"
        ];
        Wants = [ "network-online.target" ];
        PartOf = [ "graphical-session.target" ];
        ConditionPathExists = "%h/.local/state/partner-watch/state.json";
      };
      Service = {
        ExecStart = "${cfg.package}/bin/partner-watch-desktop run --config %h/.config/partner-watch/config.toml --state %h/.local/state/partner-watch/state.json";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };
  };
}

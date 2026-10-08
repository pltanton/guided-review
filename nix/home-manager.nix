self: {
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.programs.guided-review;
  skills = "${cfg.package}/share/guided-review/skills";
in {
  options.programs.guided-review = {
    enable = lib.mkEnableOption "guided-review";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "guided-review.packages.\${system}.default";
    };

    codex.enable = lib.mkEnableOption "the guided-review skills for Codex in ~/.codex/skills";
  };

  config = lib.mkIf cfg.enable {
    home.packages = [cfg.package];

    home.file = lib.mkIf cfg.codex.enable {
      ".codex/skills/guided-review".source = "${skills}/guided-review";
      ".codex/skills/guided-selfreview".source = "${skills}/guided-selfreview";
    };
  };
}

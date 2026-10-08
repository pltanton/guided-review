self: {
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.programs.guided-review;
  names = ["guided-review" "guided-selfreview"];
  skill = name:
    if cfg.checkout == null
    then "${cfg.package}/share/guided-review/skills/${name}"
    else config.lib.file.mkOutOfStoreSymlink "${cfg.checkout}/skills/${name}";
  links = dir:
    lib.listToAttrs (map (name: {
        name = "${dir}/${name}";
        value.source = skill name;
      })
      names);
in {
  options.programs.guided-review = {
    enable = lib.mkEnableOption "guided-review";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "guided-review.packages.\${system}.default";
    };

    codex.enable = lib.mkEnableOption "the guided-review skills for Codex in ~/.codex/skills";

    claude.enable = lib.mkEnableOption ''
      the guided-review skills in ~/.claude/skills, for a Claude Code without the plugin
      (the plugin also brings the Stop hook)'';

    checkout = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "/Users/me/dev/guided-review";
      description = ''
        A working copy of guided-review to link the skills from instead of the package,
        so edits to them apply without a switch.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [cfg.package];

    home.file =
      lib.optionalAttrs cfg.codex.enable (links ".codex/skills")
      // lib.optionalAttrs cfg.claude.enable (links ".claude/skills");
  };
}

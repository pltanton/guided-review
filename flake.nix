{
  description = "guided-review: a step-by-step code review viewer for Claude Code and Codex";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = [
      "aarch64-darwin"
      "x86_64-darwin"
      "aarch64-linux"
      "x86_64-linux"
    ];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    release = (builtins.fromJSON (builtins.readFile ./.claude-plugin/plugin.json)).version;
    version = "${release}-${self.shortRev or "dirty"}";
    gr = pkgs: pkgs.callPackage ./nix/package.nix {inherit version;};
  in {
    packages = forAllSystems (pkgs: {
      default = gr pkgs;
      gr = gr pkgs;
    });

    overlays.default = final: _: {guided-review = gr final;};

    homeManagerModules.default = import ./nix/home-manager.nix self;

    devShells = forAllSystems (pkgs: {
      default = pkgs.mkShell {
        inputsFrom = [(gr pkgs)];
        packages = with pkgs; [gopls git tmux jq glab gh];
      };
    });

    formatter = forAllSystems (pkgs: pkgs.alejandra);
  };
}

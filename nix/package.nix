{
  lib,
  buildGoModule,
  git,
  version,
}:
buildGoModule {
  pname = "guided-review";
  inherit version;

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../guidedreview.go
      ../guidedreview_test.go
      ../CHANGELOG.md
      ../.claude-plugin
      ../cmd
      ../internal
      ../skills
    ];
  };

  vendorHash = "sha256-iSw2yLLz/ba2GwFlNQdU3Dj/zhlIWv7wMIcFHT9RB4I=";

  subPackages = ["cmd/gr"];

  nativeCheckInputs = [git];

  ldflags = [
    "-s"
    "-w"
  ];

  postInstall = ''
    mkdir -p $out/share/guided-review
    cp -r skills $out/share/guided-review/skills
  '';

  meta = {
    description = "Step-by-step code review viewer for Claude Code and Codex";
    homepage = "https://guided-review.pltanton.dev";
    license = lib.licenses.asl20;
    mainProgram = "gr";
  };
}

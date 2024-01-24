{ stdenv, lib, fetchurl, installShellFiles, runCommand, makeWrapper
, zip, unzip, bash, coreutils, which, gnused, gnutar, gnugrep, gzip, findutils
# updater
, python3, writeScript
# Allow to independently override the jdks used to build and run respectively
, buildJdk, runJdk
, runtimeShell, file, substituteAll, writeTextFile
, enableNixHacks ? false
}:

let
  version = "5.4.1";
  sourceRoot = ".";

  src = fetchurl {
    url = "https://github.com/bazelbuild/bazel/releases/download/${version}/bazel-${version}-dist.zip";
    hash = "sha256-3P9pNXVqp6yk/Fabsr0m4VN/Cx9tG9pfKyAPqDXMUH8=";
  };

  # Update with
  # 1. export BAZEL_SELF=$(nix-build -A bazel_5)
  # 2. update version and hash for sources above
  # 3. `eval $(nix-build -A bazel_5.updater)`
  # 4. add new dependencies from the dict in ./src-deps.json if required by failing build
  srcDeps = lib.attrsets.attrValues srcDepsSet;
  srcDepsSet =
    let
      srcs = lib.importJSON ./src-deps.json;
      toFetchurl = d: lib.attrsets.nameValuePair d.name (fetchurl {
        urls = d.urls;
        sha256 = d.sha256;
        });
        in builtins.listToAttrs (map toFetchurl [
      srcs.desugar_jdk_libs
      srcs.io_bazel_skydoc
      srcs.bazel_skylib
      srcs.bazelci_rules
      srcs.io_bazel_rules_sass
      srcs.platforms
      srcs."remote_java_tools_for_testing"
      srcs."coverage_output_generator-v2.5.zip"
      srcs.build_bazel_rules_nodejs
      srcs."android_tools_pkg-0.23.0.tar.gz"
      srcs.bazel_toolchains
      srcs.com_github_grpc_grpc
      srcs.upb
      srcs.com_google_protobuf
      srcs.rules_pkg
      srcs.rules_cc
      srcs.rules_java
      srcs.rules_proto
      srcs.com_google_absl
      srcs.com_googlesource_code_re2
      srcs.com_github_cares_cares
      ]);

  distDir = runCommand "bazel-deps" {} ''
    mkdir -p $out
    for i in ${builtins.toString srcDeps}; do cp $i $out/$(stripHash $i); done
  '';

  defaultShellUtils =
    [ bash coreutils file findutils gnugrep gnused gnutar gzip python3 unzip which zip ];

  defaultShellPath = lib.makeBinPath defaultShellUtils;

  system = if stdenv.hostPlatform.isDarwin then "darwin" else "linux";

  # on aarch64 Darwin, `uname -m` returns "arm64"
  arch = with stdenv.hostPlatform; if isDarwin && isAarch64 then "arm64" else parsed.cpu.name;

  bazelRC = writeTextFile {
    name = "bazel-rc";
    text = ''
      startup --server_javabase=${runJdk}

      # Can't use 'common'; https://github.com/bazelbuild/bazel/issues/3054
      # Most commands inherit from 'build' anyway.
      build --distdir=${distDir}
      fetch --distdir=${distDir}
      query --distdir=${distDir}

      build --extra_toolchains=@bazel_tools//tools/jdk:nonprebuilt_toolchain_definition
      build --tool_java_runtime_version=local_jdk_11
      build --java_runtime_version=local_jdk_11

      # load default location for the system wide configuration
      try-import /etc/bazel.bazelrc
    '';
  };

in
stdenv.mkDerivation rec {
  pname = "bazel";
  inherit version;
  inherit src;
  inherit sourceRoot;
  patches = [
    # --experimental_strict_action_env (which may one day become the default
    # see bazelbuild/bazel#2574) hardcodes the default
    # action environment to a non hermetic value (e.g. "/usr/local/bin").
    # This is non hermetic on non-nixos systems. On NixOS, bazel cannot find the required binaries.
    # So we are replacing this bazel paths by defaultShellPath,
    # improving hermeticity and making it work in nixos.
    (substituteAll {
      src = ./patches/strict_action_env.patch;
      strictActionEnvPatch = defaultShellPath;
    })

    (substituteAll {
      src = ./patches/actions_path.patch;
      actionsPathPatch = defaultShellPath;
    })

    # bazel reads its system bazelrc in /etc
    # override this path to a builtin one
    (substituteAll {
      src = ./patches/bazel_rc.patch;
      bazelSystemBazelRCPath = bazelRC;
    })

    ./patches/nix-hacks.patch
  ];

  src_for_updater = stdenv.mkDerivation {
    name = "updater-sources";
    inherit src;
    nativeBuildInputs = [ unzip ];
    inherit sourceRoot;
    installPhase = ''
      runHook preInstall

      cp -r . "$out"

      runHook postInstall
    '';
  };
  # update the list of workspace dependencies
  passthru.updater = writeScript "update-bazel-deps.sh" ''
    #!${runtimeShell}
    (cd "${src_for_updater}" &&
        BAZEL_USE_CPP_ONLY_TOOLCHAIN=1 \
        "$BAZEL_SELF"/bin/bazel \
            query 'kind(http_archive, //external:*) + kind(http_file, //external:*) + kind(distdir_tar, //external:*) + kind(git_repository, //external:*)' \
            --loading_phase_threads=1 \
            --output build) \
    | "${python3}"/bin/python3 "${./update-srcDeps.py}" \
      "${builtins.toString ./src-deps.json}"
  '';

  # Necessary for the tests to pass on Darwin with sandbox enabled.
  # Bazel starts a local server and needs to bind a local address.
  __darwinAllowLocalNetworking = true;

  postPatch = let
    genericPatches = ''
      # Substitute j2objc and objc wrapper's python shebang to plain python path.
      substituteInPlace tools/j2objc/j2objc_header_map.py --replace "$!/usr/bin/python2.7" "#!${python3.interpreter}"
      substituteInPlace tools/j2objc/j2objc_wrapper.py --replace "$!/usr/bin/python2.7" "#!${python3.interpreter}"
      substituteInPlace tools/objc/j2objc_dead_code_pruner.py --replace "$!/usr/bin/python2.7" "#!${python3.interpreter}"

      # md5sum is part of coreutils
      sed -i 's|/sbin/md5|md5sum|g' \
        src/BUILD third_party/ijar/test/testenv.sh tools/objc/libtool.sh

      # replace initial value of pythonShebang variable in BazelPythonSemantics.java
      substituteInPlace src/main/java/com/google/devtools/build/lib/bazel/rules/python/BazelPythonSemantics.java \
        --replace '"#!/usr/bin/env " + pythonExecutableName' "\"#!${python3}/bin/python\""

      substituteInPlace src/main/java/com/google/devtools/build/lib/starlarkbuildapi/python/PyRuntimeInfoApi.java \
        --replace '"#!/usr/bin/env python3"' "\"#!${python3}/bin/python\""

      # substituteInPlace is rather slow, so prefilter the files with grep
      grep -rlZ /bin/ src/main/java/com/google/devtools | while IFS="" read -r -d "" path; do
        # If you add more replacements here, you must change the grep above!
        # Only files containing /bin are taken into account.
        substituteInPlace "$path" \
          --replace /bin/bash ${bash}/bin/bash \
          --replace "/usr/bin/env bash" ${bash}/bin/bash \
          --replace "/usr/bin/env python" ${python3}/bin/python \
          --replace /usr/bin/env ${coreutils}/bin/env \
          --replace /bin/true ${coreutils}/bin/true
      done

      grep -rlZ /bin/ tools/python | while IFS="" read -r -d "" path; do
        substituteInPlace "$path" \
          --replace "/usr/bin/env python2" ${python3.interpreter} \
          --replace "/usr/bin/env python3" ${python3}/bin/python \
          --replace /usr/bin/env ${coreutils}/bin/env
      done

      # bazel test runner include references to /bin/bash
      substituteInPlace tools/build_rules/test_rules.bzl \
        --replace /bin/bash ${bash}/bin/bash

      for i in $(find tools/cpp/ -type f)
      do
        substituteInPlace $i \
          --replace /bin/bash ${bash}/bin/bash
      done

      # Fixup scripts that generate scripts. Not fixed up by patchShebangs below.
      substituteInPlace scripts/bootstrap/compile.sh \
          --replace /bin/bash ${bash}/bin/bash

      # add nix environment vars to .bazelrc
      cat >> .bazelrc <<EOF
      # Limit the resources Bazel is allowed to use during the build to 1/2 the
      # available RAM and 3/4 the available CPU cores. This should help avoid
      # overwhelming the build machine.
      build --local_ram_resources=HOST_RAM*.5
      build --local_cpu_resources=HOST_CPUS*.75

      build --distdir=${distDir}
      fetch --distdir=${distDir}
      build --copt="$(echo $NIX_CFLAGS_COMPILE | sed -e 's/ /" --copt="/g')"
      build --host_copt="$(echo $NIX_CFLAGS_COMPILE | sed -e 's/ /" --host_copt="/g')"
      build --linkopt="$(echo $(< ${stdenv.cc}/nix-support/libcxx-ldflags) | sed -e 's/ /" --linkopt="/g')"
      build --host_linkopt="$(echo $(< ${stdenv.cc}/nix-support/libcxx-ldflags) | sed -e 's/ /" --host_linkopt="/g')"
      build --linkopt="-Wl,$(echo $NIX_LDFLAGS | sed -e 's/ /" --linkopt="-Wl,/g')"
      build --host_linkopt="-Wl,$(echo $NIX_LDFLAGS | sed -e 's/ /" --host_linkopt="-Wl,/g')"
      build --extra_toolchains=@bazel_tools//tools/jdk:nonprebuilt_toolchain_definition
      build --verbose_failures
      build --curses=no
      build --features=-layering_check
      EOF

      cat >> tools/jdk/BUILD.tools <<EOF
      load("@bazel_tools//tools/jdk:default_java_toolchain.bzl", "default_java_toolchain", "NONPREBUILT_TOOLCHAIN_CONFIGURATION")
      default_java_toolchain(
        name = "nonprebuilt_toolchain",
        configuration = NONPREBUILT_TOOLCHAIN_CONFIGURATION,
        java_runtime = "@local_jdk//:jdk",
      )
      EOF

      cat >> third_party/grpc/bazel_1.41.0.patch <<EOF
      diff --git a/third_party/grpc/BUILD b/third_party/grpc/BUILD
      index 39ee9f97c6..9128d20c85 100644
      --- a/third_party/grpc/BUILD
      +++ b/third_party/grpc/BUILD
      @@ -28,7 +28,6 @@ licenses(["notice"])
       package(
           default_visibility = ["//visibility:public"],
           features = [
      -        "layering_check",
               "-parse_headers",
           ],
       )
      EOF

      # add the same environment vars to compile.sh
      sed -e "/\$command \\\\$/a --copt=\"$(echo $NIX_CFLAGS_COMPILE | sed -e 's/ /" --copt=\"/g')\" \\\\" \
          -e "/\$command \\\\$/a --host_copt=\"$(echo $NIX_CFLAGS_COMPILE | sed -e 's/ /" --host_copt=\"/g')\" \\\\" \
          -e "/\$command \\\\$/a --linkopt=\"$(echo $(< ${stdenv.cc}/nix-support/libcxx-ldflags) | sed -e 's/ /" --linkopt=\"/g')\" \\\\" \
          -e "/\$command \\\\$/a --host_linkopt=\"$(echo $(< ${stdenv.cc}/nix-support/libcxx-ldflags) | sed -e 's/ /" --host_linkopt=\"/g')\" \\\\" \
          -e "/\$command \\\\$/a --linkopt=\"-Wl,$(echo $NIX_LDFLAGS | sed -e 's/ /" --linkopt=\"-Wl,/g')\" \\\\" \
          -e "/\$command \\\\$/a --host_linkopt=\"-Wl,$(echo $NIX_LDFLAGS | sed -e 's/ /" --host_linkopt=\"-Wl,/g')\" \\\\" \
          -e "/\$command \\\\$/a --tool_java_runtime_version=local_jdk_11 \\\\" \
          -e "/\$command \\\\$/a --java_runtime_version=local_jdk_11 \\\\" \
          -e "/\$command \\\\$/a --verbose_failures \\\\" \
          -e "/\$command \\\\$/a --curses=no \\\\" \
          -e "/\$command \\\\$/a --features=-layering_check \\\\" \
          -i scripts/bootstrap/compile.sh

      # This is necessary to avoid:
      # "error: no visible @interface for 'NSDictionary' declares the selector
      # 'initWithContentsOfURL:error:'"
      # This can be removed when the apple_sdk is upgraded beyond 10.13+
      sed -i '/initWithContentsOfURL:versionPlistUrl/ {
        N
        s/error:nil\];/\];/
      }' tools/osx/xcode_locator.m

      # append the PATH with defaultShellPath in tools/bash/runfiles/runfiles.bash
      echo "PATH=\$PATH:${defaultShellPath}" >> runfiles.bash.tmp
      cat tools/bash/runfiles/runfiles.bash >> runfiles.bash.tmp
      mv runfiles.bash.tmp tools/bash/runfiles/runfiles.bash

      patchShebangs .
    '';
    in genericPatches;

  buildInputs = [ buildJdk defaultShellUtils ];

  # when a command can’t be found in a bazel build, you might also
  # need to add it to `defaultShellPath`.
  nativeBuildInputs = [ python3.pkgs.absl-py makeWrapper installShellFiles ];

  # Bazel makes extensive use of symlinks in the WORKSPACE.
  # This causes problems with infinite symlinks if the build output is in the same location as the
  # Bazel WORKSPACE. This is why before executing the build, the source code is moved into a
  # subdirectory.
  # Failing to do this causes "infinite symlink expansion detected"
  preBuildPhases = ["preBuildPhase"];
  preBuildPhase = ''
    mkdir bazel_src
    shopt -s dotglob extglob
    mv !(bazel_src) bazel_src
  '';
  buildPhase = ''
    runHook preBuild

    # Increasing memory during compilation might be necessary.
    # export BAZEL_JAVAC_OPTS="-J-Xmx2g -J-Xms200m"

    # If EMBED_LABEL isn't set, it'd be auto-detected from CHANGELOG.md
    # and `git rev-parse --short HEAD` which would result in
    # "3.7.0- (@non-git)" due to non-git build and incomplete changelog.
    # Actual bazel releases use scripts/release/common.sh which is based
    # on branch/tag information which we don't have with tarball releases.
    # Note that .bazelversion is always correct and is based on bazel-*
    # executable name, version checks should work fine
    export EMBED_LABEL="${version}- (@non-git)"
    ${bash}/bin/bash ./bazel_src/compile.sh
    ./bazel_src/scripts/generate_bash_completion.sh \
        --bazel=./bazel_src/output/bazel \
        --output=./bazel_src/output/bazel-complete.bash \
        --prepend=./bazel_src/scripts/bazel-complete-header.bash \
        --prepend=./bazel_src/scripts/bazel-complete-template.bash
    ${python3}/bin/python3 ./bazel_src/scripts/generate_fish_completion.py \
        --bazel=./bazel_src/output/bazel \
        --output=./bazel_src/output/bazel-complete.fish

    # need to change directory for bazel to find the workspace
    cd ./bazel_src
    # build execlog tooling
    export HOME=$(mktemp -d)
    ./output/bazel build  src/tools/execlog:parser_deploy.jar
    cd -

    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    mkdir -p $out/bin

    # official wrapper scripts that searches for $WORKSPACE_ROOT/tools/bazel
    # if it can’t find something in tools, it calls $out/bin/bazel-{version}-{os_arch}
    # The binary _must_ exist with this naming if your project contains a .bazelversion
    # file.
    cp ./bazel_src/scripts/packages/bazel.sh $out/bin/bazel
    wrapProgram $out/bin/bazel $wrapperfile --suffix PATH : ${defaultShellPath}
    mv ./bazel_src/output/bazel $out/bin/bazel-${version}-${system}-${arch}

    mkdir $out/share
    cp ./bazel_src/bazel-bin/src/tools/execlog/parser_deploy.jar $out/share/parser_deploy.jar
    cat <<EOF > $out/bin/bazel-execlog
    #!${runtimeShell} -e
    ${runJdk}/bin/java -jar $out/share/parser_deploy.jar \$@
    EOF
    chmod +x $out/bin/bazel-execlog

    # shell completion files
    installShellCompletion --bash \
      --name bazel.bash \
      ./bazel_src/output/bazel-complete.bash
    installShellCompletion --zsh \
      --name _bazel \
      ./bazel_src/scripts/zsh_completion/_bazel
    installShellCompletion --fish \
      --name bazel.fish \
      ./bazel_src/output/bazel-complete.fish
  '';

  # Install check fails on `aarch64-darwin`
  # https://github.com/NixOS/nixpkgs/issues/145587
  doInstallCheck = stdenv.hostPlatform.system != "aarch64-darwin";
  installCheckPhase = ''
    export TEST_TMPDIR=$(pwd)

    hello_test () {
      $out/bin/bazel test \
        --test_output=errors \
        examples/cpp:hello-success_test \
        examples/java-native/src/test/java/com/example/myproject:hello
    }

    cd ./bazel_src
    rm .bazelversion # this doesn't necessarily match the version we built

    # test whether $WORKSPACE_ROOT/tools/bazel works

    mkdir -p tools
    cat > tools/bazel <<"EOF"
    #!${runtimeShell} -e
    exit 1
    EOF
    chmod +x tools/bazel

    # first call should fail if tools/bazel is used
    ! hello_test

    cat > tools/bazel <<"EOF"
    #!${runtimeShell} -e
    exec "$BAZEL_REAL" "$@"
    EOF

    # second call succeeds because it defers to $out/bin/bazel-{version}-{os_arch}
    hello_test

    runHook postInstall
  '';

  # Save paths to hardcoded dependencies so Nix can detect them.
  # This is needed because the templates get tar’d up into a .jar.
  postFixup = ''
    mkdir -p $out/nix-support
    echo "${defaultShellPath}" >> $out/nix-support/depends
    # The string literal specifying the path to the bazel-rc file is sometimes
    # stored non-contiguously in the binary due to gcc optimisations, which leads
    # Nix to miss the hash when scanning for dependencies
    echo "${bazelRC}" >> $out/nix-support/depends
  '';

  dontStrip = true;
  dontPatchELF = true;
}

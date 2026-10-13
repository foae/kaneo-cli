# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "4.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.0/kaneo-cli_4.0.0_darwin_arm64.tar.gz"
      sha256 "f71745ae2abef31120eda0070f0f1a910eb65744874512270eb8424b428dad7e"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.0/kaneo-cli_4.0.0_darwin_amd64.tar.gz"
      sha256 "64f925e07ba1e4afc65cab667ece720ccc0df52f398b7c0429d385137aad5ed0"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.0/kaneo-cli_4.0.0_linux_arm64.tar.gz"
      sha256 "e9cbbf9d5a83523da928c387f32c633c7544ccdc5dde473cd03d726cca2b11a4"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.0/kaneo-cli_4.0.0_linux_amd64.tar.gz"
      sha256 "7eda98bdbdd24ffb0f814543c0edf08977fff14bd01053460ae9601282079a6a"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

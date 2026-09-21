# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.3.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.0/kaneo-cli_1.3.0_darwin_arm64.tar.gz"
      sha256 "94ed879b320f74481446169266ed1ccd89078206ad78240a3afb660650d196ce"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.0/kaneo-cli_1.3.0_darwin_amd64.tar.gz"
      sha256 "11ef2f11303d761af808319c202a1fca338dc21bfaadf436c9d2eb2559c43605"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.0/kaneo-cli_1.3.0_linux_arm64.tar.gz"
      sha256 "089b770938b40e797090e13611102fa136fc6b334c6b14877278f8c8f7866d64"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.0/kaneo-cli_1.3.0_linux_amd64.tar.gz"
      sha256 "7f3c1dbea08370e02a3ad1a7ed25f83d7347b3b22ce96295386efb583a810321"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

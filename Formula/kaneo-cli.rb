# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.6.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.0/kaneo-cli_1.6.0_darwin_arm64.tar.gz"
      sha256 "a84ff1963eac41eb003140308b2c686f2971a2b58c6d08f80f99f4771945a732"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.0/kaneo-cli_1.6.0_darwin_amd64.tar.gz"
      sha256 "e873f1f208c5857c6647914466dc5d6ce2a407d3d8e6ad3cfb004f54277b65c4"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.0/kaneo-cli_1.6.0_linux_arm64.tar.gz"
      sha256 "d7e805af0151009b607d3d218471d917e8780dd47c9d79e72cdf67c9b7322483"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.6.0/kaneo-cli_1.6.0_linux_amd64.tar.gz"
      sha256 "5cefd6507746d12914de54d2001842d19819f5542391fcae6b285c53e9dfc7c6"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.3.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.1/kaneo-cli_1.3.1_darwin_arm64.tar.gz"
      sha256 "ee0828e018e20bf653ac80146da7de9771e29b0e27cee78c804f2b9a22c52d41"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.1/kaneo-cli_1.3.1_darwin_amd64.tar.gz"
      sha256 "c1b256ac9e90f1787038934c4a75c84dcc161fad0f3c14c3b3bf969aeb7e2beb"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.1/kaneo-cli_1.3.1_linux_arm64.tar.gz"
      sha256 "9d69ec055ee1606c04b7b7098b3b140b75f6f2d531c7dc60f53a7cb70968530e"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.3.1/kaneo-cli_1.3.1_linux_amd64.tar.gz"
      sha256 "e0826072bceda7611a07fa192ff34edb0d5ea4038efa7756492f3b00fa19b986"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

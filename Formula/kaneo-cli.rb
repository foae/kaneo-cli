# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.0.0/kaneo-cli_1.0.0_darwin_arm64.tar.gz"
      sha256 "7be704fb4e81647a8087455b1b5a2b5537c12102aa44dcead4380f8ec1fd124c"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.0.0/kaneo-cli_1.0.0_darwin_amd64.tar.gz"
      sha256 "6c90839cb78c1c7e5cd505da32b07ac785f7368aff6ccf0e454ed1377691b080"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.0.0/kaneo-cli_1.0.0_linux_arm64.tar.gz"
      sha256 "4bfce0615b6501ddedbae83c03559fede06bc31369b696c4df06a86464d878c0"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.0.0/kaneo-cli_1.0.0_linux_amd64.tar.gz"
      sha256 "7cd103cf24c8355aa224d7987635a6d8e73636826051460f5798a6cf7d399573"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "4.0.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.1/kaneo-cli_4.0.1_darwin_arm64.tar.gz"
      sha256 "2be6e562fab1f60512d3e85bc93c45b8089240213e0b71930c22ee6bc7ae07c7"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.1/kaneo-cli_4.0.1_darwin_amd64.tar.gz"
      sha256 "f2220437d38c50f2a2ecea642707baffd9f257be052067a1adda50b10764bcea"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.1/kaneo-cli_4.0.1_linux_arm64.tar.gz"
      sha256 "8921bc062f8c8548b809a451879be1ab4fa02bc176d9977f59b49c1900421210"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v4.0.1/kaneo-cli_4.0.1_linux_amd64.tar.gz"
      sha256 "e48fdbf6327ab2d52ca4a20b8bd6c128c5277eea97f0823ad6eea64978674e73"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

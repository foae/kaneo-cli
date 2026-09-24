# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.7.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.7.0/kaneo-cli_1.7.0_darwin_arm64.tar.gz"
      sha256 "28d6ab2d5f1588241cfde5d77d2016e759eb507798f7a26d71ca77246d3773d1"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.7.0/kaneo-cli_1.7.0_darwin_amd64.tar.gz"
      sha256 "144624596849f2f2a9b51583c9e33660fe7aede5f82a9375c0189e5bfa3f39b1"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.7.0/kaneo-cli_1.7.0_linux_arm64.tar.gz"
      sha256 "ac77f15ea83926e6626749d5bde76e88fba775dcc76f89e33d06c6f883e1cb40"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.7.0/kaneo-cli_1.7.0_linux_amd64.tar.gz"
      sha256 "7c5213e2ec789c43e9122358af1fd7987784bf5774949f453253a1aa4eff8e2a"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

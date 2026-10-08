# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "3.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v3.0.0/kaneo-cli_3.0.0_darwin_arm64.tar.gz"
      sha256 "3d6680d7372be974556cf63647a47fc29cd3ec29ff9ea885027256e22c15ab0b"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v3.0.0/kaneo-cli_3.0.0_darwin_amd64.tar.gz"
      sha256 "c444db4535f7582dccf1edc7e073a904e657d30392925a7c57b1ca46fe39e1e0"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v3.0.0/kaneo-cli_3.0.0_linux_arm64.tar.gz"
      sha256 "88d7e86823efbb6e4459fd0e882a5ca6d0faccf43f4d883f8f66777da26e151e"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v3.0.0/kaneo-cli_3.0.0_linux_amd64.tar.gz"
      sha256 "ee65239633b0d49016611e9d0ba7a0312cda353f1f09796d2590e34c5b362651"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

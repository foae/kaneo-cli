# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.8.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.8.0/kaneo-cli_1.8.0_darwin_arm64.tar.gz"
      sha256 "fbfa6ae7543e79bc3a5bad2514626ce4f21dee90894f2d4b5c42252915f6792a"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.8.0/kaneo-cli_1.8.0_darwin_amd64.tar.gz"
      sha256 "1a26b8d5d7ca3c35c4b82bc7433d0ef2dc126eeb6d96bed2a12458819f5c77c4"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.8.0/kaneo-cli_1.8.0_linux_arm64.tar.gz"
      sha256 "b70b761e8de79a8843bdebc30d0bcd58b973aa1abfde2cdad36f98cc6c07d9ae"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.8.0/kaneo-cli_1.8.0_linux_amd64.tar.gz"
      sha256 "b5729129b6cb58eb514ec55c1eb9bd18df0c5145274f8d4b21ff4b2ba75cd6b6"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

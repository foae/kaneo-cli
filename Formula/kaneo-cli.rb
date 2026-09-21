# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.4.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.4.0/kaneo-cli_1.4.0_darwin_arm64.tar.gz"
      sha256 "500419c825ff562c43f58d29176f398887d3ae12300bb724b2bdeb1c9a2d91c2"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.4.0/kaneo-cli_1.4.0_darwin_amd64.tar.gz"
      sha256 "5f2fe2bcff3b0a166fea55cf879bc5d276e3394baa85599ceb998dc02317c0be"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.4.0/kaneo-cli_1.4.0_linux_arm64.tar.gz"
      sha256 "da0da406fe0dc6ebd60dcf14e1fd12d1c0fea4191fbf92e033e3bd5238ea1cc9"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.4.0/kaneo-cli_1.4.0_linux_amd64.tar.gz"
      sha256 "d8f348f4e09d7fe932ad2286edaf1d7364700b5fc2b4f7287a75da29106c9ffd"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

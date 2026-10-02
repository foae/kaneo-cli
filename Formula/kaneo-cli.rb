# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "2.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v2.0.0/kaneo-cli_2.0.0_darwin_arm64.tar.gz"
      sha256 "c86c70181e0d951019f706e91ac09f78ab3db6636b2ae0061d052cca166334f5"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v2.0.0/kaneo-cli_2.0.0_darwin_amd64.tar.gz"
      sha256 "e84c9ad0d1d340a59113aa477e8b471ff81fec7c618cd42e45ed97639d2c6d94"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v2.0.0/kaneo-cli_2.0.0_linux_arm64.tar.gz"
      sha256 "fb720ace52b9ef3bd02abd1d4347c3d75660493a688f9e0bf113edc885150515"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v2.0.0/kaneo-cli_2.0.0_linux_amd64.tar.gz"
      sha256 "9027c5dfa69e2646e18e8ced29a0f6332470af7e3d04288d7c5ef859717f3318"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

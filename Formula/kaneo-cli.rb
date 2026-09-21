# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.1.0/kaneo-cli_1.1.0_darwin_arm64.tar.gz"
      sha256 "f3bdc0e526bdc1b59b2051e470d05d901451a4adfb6c74eafa7c6c708c61d7ba"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.1.0/kaneo-cli_1.1.0_darwin_amd64.tar.gz"
      sha256 "bb091003d02d2328e44e2f5d5a757be6d624a2179f69b3cdbb469dd911d3ed67"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.1.0/kaneo-cli_1.1.0_linux_arm64.tar.gz"
      sha256 "34e159c78290d064db63b0b6b65790ef6cb7aadcfe038eca062c16c6d8b6b070"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.1.0/kaneo-cli_1.1.0_linux_amd64.tar.gz"
      sha256 "f91fe6d4858ee7256dc42aec760c2788c964340b1cd154e49e67742a4d750eed"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

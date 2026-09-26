# typed: false
# This file is generated from published release checksums. Do not edit by hand.
# Tap: https://github.com/foae/kaneo-cli
class KaneoCli < Formula
  desc "Command-line client for Kaneo"
  homepage "https://github.com/foae/kaneo-cli"
  version "1.9.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.9.0/kaneo-cli_1.9.0_darwin_arm64.tar.gz"
      sha256 "862d8f58579662788d99cd12b6f8e5fdd9552c39e059521063af21ff252ea8b5"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.9.0/kaneo-cli_1.9.0_darwin_amd64.tar.gz"
      sha256 "90d6e8625822bed42b2fdb113082ec26443018f1665e6363dcbafc84bde17f8c"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/foae/kaneo-cli/releases/download/v1.9.0/kaneo-cli_1.9.0_linux_arm64.tar.gz"
      sha256 "176aba4cee1ad7ac9d3cf9290d22e139c9cd71920315e92a74e09d41dd7efd61"
    else
      url "https://github.com/foae/kaneo-cli/releases/download/v1.9.0/kaneo-cli_1.9.0_linux_amd64.tar.gz"
      sha256 "0a04421eccc6325ad6c28278d9348ded28b5c1f820c50efcb426669330809f06"
    end
  end

  def install
    bin.install "kaneo-cli"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kaneo-cli version")
  end
end

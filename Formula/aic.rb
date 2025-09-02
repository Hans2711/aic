class Aic < Formula
  desc "AI-assisted git commit message generator"
  homepage "https://github.com/Hans2711/aic"
  url "https://github.com/Hans2711/aic/archive/dad708eb1004cf24954193adca38cbc00f26aee0.tar.gz"
  sha256 "70577b15d7c764de9905895da1583c0750ace9aff6bbb2e9adca1319dcc39c5d"
  version "1.0.0"
  head "https://github.com/Hans2711/aic.git", branch: "master"

  depends_on "go" => :build

  def install
    # Prevent the Go tool from downloading a different toolchain in Homebrew's sandbox.
    ENV["GOTOOLCHAIN"] = "local"
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/aic"
  end

  test do
    out = shell_output("#{bin}/aic --version")
    assert_match "aic #{version}", out
  end
end

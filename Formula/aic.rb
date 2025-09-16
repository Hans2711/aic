class Aic < Formula
  release_env_path = File.expand_path("../release.env", __dir__)
  raise "release.env not found at #{release_env_path}" unless File.exist?(release_env_path)

  release_metadata = {}
  File.foreach(release_env_path) do |line|
    line = line.strip
    next if line.empty? || line.start_with?("#")

    key, value = line.split("=", 2)
    next if value.nil?

    release_metadata[key] = value.strip
  end

  release_version = release_metadata.fetch("VERSION")
  source_url = "https://github.com/Hans2711/aic/archive/refs/tags/v#{release_version}.tar.gz"
  sha_value = release_metadata["SOURCE_SHA256"]

  desc "AI-assisted git commit message generator"
  homepage "https://github.com/Hans2711/aic"
  url source_url
  version release_version
  sha256 sha_value.nil? || sha_value.empty? ? :no_check : sha_value
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

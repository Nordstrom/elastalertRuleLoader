#!/usr/bin/env rake

@current_dir = File.dirname(__FILE__)

require 'bundler/setup'
require 'sf-utilities/rake-tasks'

LOG = Logger.new(STDOUT)

@name = "elastalertruleloader"
@current_dir = File.dirname(__FILE__)
@build_dir = File.join(@current_dir, 'build')
@version_file = File.join(@current_dir, 'build', 'version')
@version = { major: '1', minor: '0'}
@golang_version = '1.6'


def validate_golang_version
  version = `go version`.chomp
  raise "Golang is not the correct version, expected #{@golang_version}, got '#{version}'" unless version.include?(@golang_version)
end

def git_commit_hash
  commit = `git rev-parse --short HEAD`.strip
  local_changes = `git status --porcelain`
  changes = ""
  "#{commit}#{changes}" 
end

def is_git_repository?
  Dir.exist?('.git')
end

def get_os
  case RUBY_PLATFORM
  when /darwin/
    return 'darwin'
  else
    return 'linux'
  end
end

def create_directory(dir)
  Dir.mkdir(dir) unless File.exists?(dir)
end

def run_command(cmd, runner = Mixlib::ShellOut)
  LOG.info("Running command: #{cmd}")

  command = runner.new(cmd)
  command.live_stdout = STDOUT
  command.live_stderr = STDERR
  command.run_command
  command.error!
end

def version
  if is_git_repository?
    version = "#{@version[:major]}.#{@version[:minor]}.#{git_commit_hash}"
    create_version_file(version)
  else
    version = retrive_version_from_file
  end

  version
end

def retrive_version_from_file
  IO.read(@version_file)
end

def create_version_file(version)
  create_directory(@build_dir)
  File.write(@version_file, version)
end

task :build_app do
  LOG.progname = 'build_app'

  LOG.info('Validating golang version')
  validate_golang_version

  create_directory(@build_dir)
  output_file = File.join(@build_dir, @name)

  commit_hash = git_commit_hash
  # build target
  run_command(%Q{GOOS=linux GOARCH=amd64 go build -o #{output_file} -ldflags "-X main.Version=#{version}"})
  # build local for non-linux OS
  os = get_os
  if os != "linux" 
    output_file = "#{output_file}_local"
    run_command(%Q{GOOS=#{os} GOARCH=amd64 go build -o #{output_file} -ldflags "-X main.Version=#{version}"})
  end

  File.chmod(0744, output_file)
end

task :unit_test do
  LOG.progname = 'unit_test'
  run_command('go test -v -timeout=5s $(go list ./... | grep -v "/vendor/")')
end

task :build_image do
  raise 'Dockerfile not found, please create one before running build image' unless File.exist?('Dockerfile')

  create_directory(build_dir)
  FileUtils.cp('Dockerfile', build_dir)

  build_args = @docker_build_args.map do |key, val|
    "--build-arg #{key}=#{val}"
  end.join(' ')

  run_command("docker build --no-cache #{build_args} -t #{docker_image} #{build_dir}")
end

def release_image
  run_command("docker push #{docker_image}")
end

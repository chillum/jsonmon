# frozen_string_literal: true

require 'go4rake'
ENV['GOARM'] = '7'

task default: %i[zip docker]

desc 'Update AngularJS from node_modules'
task :angular do
  FileUtils.cp 'node_modules/angular/angular.min.js', 'ui/', preserve: true, verbose: true
end

desc 'Update bundled Web UI (run this after modifying it)'
task :bindata do
  sh 'go-bindata -nocompress -nomemcopy -mode 0644 -prefix ui ui'
end

desc 'Build Docker image'
task :docker do
  ENV['GOOS']   = 'linux'
  ENV['GOARCH'] = 'amd64'
  sh 'go build'

  sh 'docker build -t chillum/jsonmon .'
end

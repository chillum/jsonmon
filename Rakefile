require 'go4rake'

task :default => [:zip, :docker]

desc 'Update bundled Web UI (run this after modifying the UI)'
task :ui do
  sh 'go-bindata -nocompress -nomemcopy -prefix ui/html ui/html'
end

desc 'Build Docker image'
task :docker do
  ENV['GOOS']   = 'linux'
  ENV['GOARCH'] = 'amd64'
  sh 'go build'

  sh 'docker build -t chillum/jsonmon .'
end

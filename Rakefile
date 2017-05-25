require 'go4rake'

task :default => :zip

desc 'Update bundled Web UI (run this after modifying the UI)'
task :ui do
  sh 'go-bindata -nocompress -nomemcopy -prefix ui/html ui/html'
end

desc 'Build Docker image'
task :docker do
  sh 'GOOS=linux GOARCH=amd64 go build'
  sh 'docker build -t jsonmon .'
end

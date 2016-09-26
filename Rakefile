require 'go4rake'

task :default => :zip

desc 'Update the Web UI'
task :ui do
  `svn co --force https://github.com/chillum/jsonmon-ui/trunk/html ui`
  `go-bindata -nometadata -nocompress -nomemcopy -prefix ui ui`
end

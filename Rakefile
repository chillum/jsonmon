require 'go4rake'

task :default => :zip

desc 'Update bundled Web UI (run this after modifying the UI)'
task :ui do
  `go-bindata -nocompress -nomemcopy -prefix ui/html ui/html`
end

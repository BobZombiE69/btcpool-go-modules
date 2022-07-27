# Stratum Switcher

A Stratum agent that automatically switches between Stratum servers in different currencies based on external commands (values ​​in a specific path under Zookeeper).

### build & run

Installgolang

```bash
mkdir ~/source
cd ~/source
wget http://storage.googleapis.com/golang/go1.10.3.linux-amd64.tar.gz
cd /usr/local
tar zxf ~/source/go1.10.3.linux-amd64.tar.gz
ln -s /usr/local/go/bin/go /usr/local/bin/go
```

Construct

```bash
mkdir -p /work/golang
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get github.com/BobZombiE69/btcpool-go-modules/wstratumSwitcher
```

Generate installation package (optional)

```bash
cd $GOPATH/src/github.com/BobZombiE69/btcpool-go-modules/stratumSwitcher
mkdir build
cd build
cmake ..
make package
```

Edit configuration file

```bash
mkdir /work/golang/stratumSwitcher
mkdir /work/golang/stratumSwitcher/log
cp /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/stratumSwitcher/config.default.json /work/golang/stratumSwitcher/config.json
vim /work/golang/stratumSwitcher/config.json
```

Create supervisor entry

```bash
vim /etc/supervisor/conf.d/switcher.conf
```

```conf
[program:switcher]
directory=/work/golang/stratumSwitcher
command=/work/golang/bin/stratumSwitcher -config=/work/golang/stratumSwitcher/config.json -log_dir=/work/golang/stratumSwitcher/log -v 2
autostart=true
autorestart=true
startsecs=6
startretries=20

redirect_stderr=true
stdout_logfile_backups=5
stdout_logfile=/work/golang/stratumSwitcher/log/stdout.log
```

Change the number of supervisor file descriptors (that is, the maximum number of TCP connections)
```bash
sed -i "s/\\[supervisord\\]/[supervisord]\nminfds=65535/" /etc/supervisor/supervisord.conf
service supervisor restart
```

run

```bash
supervisorctl reread
supervisorctl update
supervisorctl status
```

#### 更新

```bash
export GOPATH=/work/golang
GIT_TERMINAL_PROMPT=1 go get -u github.com/BobZombiE69/btcpool-go-modules/stratumSwitcher
diff /work/golang/src/github.com/BobZombiE69/btcpool-go-modules/stratumSwitcher/config.default.json /work/golang/stratumSwitcher/config.json
```

##### graceful restart/hot update (experimental)

This feature can be used to upgrade stratumSwitcher to a newer version, change the stratumSwitcher configuration to take effect, or simply restart the service. Most of the Stratum connections being proxied are not disconnected during a service restart.

Currently this feature is only available on Linux.

```bash
prlimit --nofile=327680 --pid=`supervisorctl pid switcher`
kill -USR2 `supervisorctl pid switcher`
```

The process will load the new binary on the original pid and will not generate a new pid. Before the original process exits, "./runtime.json" (including the listening port and information about all connections it is proxying) will be written for the new process to use to restore connections. Make sure the process has write permissions to its working directory.

In most cases, the new process can resume all connections that were proxied by the original process, but all connections in the authentication phase will be discarded.

Occasionally, however, some connections cannot be recovered by the new process (prompting that the file descriptor is invalid), and these connections will be disconnected at this time without causing resource leaks. The cause of the problem is that before exec is executed, calling the command to obtain the file descriptor will cause the file descriptor occupied by the process to double. Once the file descriptor exceeds the upper limit set in the supervisor, subsequent connections cannot be reserved. The `prlimit` command listed above was added to solve this problem.

The new binary will re-read the configuration file and listen on its defined port. Therefore, you can modify the configuration file to switch the listening port before graceful restart.

However, it should be noted that if the file descriptor reaches the upper limit during the reserved connection stage, the exec command may fail due to the lack of available file descriptors, and the program will crash and exit. Make sure to set enough file descriptors in the `prlimit` command.
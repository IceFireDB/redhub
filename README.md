<!--
 * @Author: gitsrc
 * @Date: 2021-09-24 15:07:31
 * @LastEditors: gitsrc
 * @LastEditTime: 2021-09-27 11:18:10
 * @FilePath: /redhub/README.md
-->
<p align="center">
    <img 
        src="https://user-images.githubusercontent.com/12872991/134626503-c022bb8e-2d5c-4760-a470-f56ff8ef036f.png" 
        border="0" alt="REDHUB">
    <br>
</p>

# RedHub
<a href="https://pkg.go.dev/github.com/IceFireDB/redhub"><img src="https://img.shields.io/badge/api-reference-blue.svg?style=flat-square" alt="GoDoc"></a>
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FIceFireDB%2Fredhub.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FIceFireDB%2Fredhub?ref=badge_shield)

High-performance RESP-Server multi-threaded framework, based on RawEpoll model.
* Ultra high performance
* Fully multi-threaded support
* Low CPU resource consumption
* Compatible with redis protocol
* Create a Redis compatible server with RawEpoll model in Go

# Installing

```
go get -u github.com/IceFireDB/redhub
```

# Example

Here is a simple framework usage example,support the following redis commands:

- SET key value
- GET key
- DEL key
- PING
- QUIT

You can run this example in terminal:

```sh
go run example/server.go
```

# Benchmarks

```
Machine information
        OS : Debian Buster 10.6 64bit 
       CPU : 8 CPU cores
    Memory : 64.0 GiB

Go Version : go1.16.5 linux/amd64

```

### 【Redis-server5.0.3】 Single-threaded, no disk persistence.

```
$ ./redis-server --port 6380 --appendonly no
```
```
$ redis-benchmark -h 127.0.0.1 -p 6380 -n 50000000 -t set,get -c 512 -P 1024 -q
SET: 2306060.50 requests per second
GET: 3096742.25 requests per second
```

### 【Redis-server6.2.5】 Single-threaded, no disk persistence.

```
$ ./redis-server --port 6380 --appendonly no
```
```
$ redis-benchmark -h 127.0.0.1 -p 6380 -n 50000000 -t set,get -c 512 -P 1024 -q
SET: 2076325.75 requests per second
GET: 2652801.50 requests per second
```

### 【Redis-server6.2.5】 Multi-threaded, no disk persistence.

```
io-threads-do-reads yes
io-threads 8
$ ./redis-server redis.conf
```
```
$ redis-benchmark -h 127.0.0.1 -p 6379 -n 50000000 -t set,get -c 512 -P 1024 -q
SET: 1944692.88 requests per second
GET: 2375184.00 requests per second
```

### 【RedCon】 Multi-threaded, no disk persistence

```
$ go run example/clone.go
```
```
$ redis-benchmark -h 127.0.0.1 -p 6380 -n 50000000 -t set,get -c 512 -P 1024 -q
SET: 2332742.25 requests per second
GET: 14654162.00 requests per second
```
### 【RedHub】 Multi-threaded, no disk persistence

```
$ go run example/server.go
```
```
$ redis-benchmark -h 127.0.0.1 -p 6380 -n 50000000 -t set,get -c 512 -P 1024 -q
SET: 4087305.00 requests per second
GET: 16490765.00 requests per second
```

<p align="center">
    <img 
        src="https://user-images.githubusercontent.com/12872991/134836128-423fd389-0fae-4e37-81c2-3b0066ed5f56.png" 
        border="0" alt="REDHUB Benchmarks">
    <br>
</p>


<p align="center">
    <img 
        src="https://user-images.githubusercontent.com/12872991/134836167-37c41c77-d77e-4ca8-96cb-4bab8ab65fa0.png" 
        border="0" alt="REDHUB Benchmarks">
    <br>
</p>


<!--
```
$ redis-benchmark -p 6380 -t set,get -n 10000000 -q -P 512 -c 512
SET: 2840909.00 requests per second
GET: 5643341.00 requests per second
```
-->

# Disclaimers
When you use this software, you have agreed and stated that the author, maintainer and contributor of this software are not responsible for any risks, costs or problems you encounter. If you find a software defect or BUG, ​​please submit a patch to help improve it!

# License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FIceFireDB%2Fredhub.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2FIceFireDB%2Fredhub?ref=badge_large)

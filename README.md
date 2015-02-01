# ernest-server
### Control & data server for the Ernest system

[Master Project Repo](https://github.com/rschlaikjer/Ernest)

## What is this
This server listens for HTTP requests from the Ernest master and logs the
environmental data that it gets to the database. It also provides the logic for
whether or not the master node should activate the furnace.

This server is very similar to the
[GoNest](https://github.com/rschlaikjer/GoNest) server, which was part of the
[ArNest](https://github.com/rschlaikjer/ArNest) project that preceded Ernest.
The major difference between the two is that this server can deal with having
multuiple nodes worth of data sent in. The user can specify which node ID should
be used for temperature data when deciding whether to turn on the furnace. **This
needs to be defined in the settings table, or the server will never update the
furnace state.**

## Fancy features
### Presence detection using DHCP
In my house the router is a computer in the basement, and so it is easy to keep
an eye on DHCP leases.  To work out whether people are home, this server tails
syslog for lines from DHCPD, and when it gets a dhcprequest from a MAC address
that it knows to be a person living in the house, it marks that person as
present. If no phones request addresses within 10 minutes, it assumes nobody is
home and drops the temperature.

This works out of the box with android phones, which ping the dhcp server
about every five minutes to confirm their address. iPhones weren't doing this,
and so people were getting marked as away even while they were still home.
As a workaround, reducing the DHCP lease time to less than ten minutes ensures
that all devuces reauth frequently enough to count as home.

### Status page / graphs
Graphs are cool, as is controlling some aspects of the thermostat from the web
(such as turning on the heat if you are freezing). To that end there's a simple
status page that shows the current config, who's home, and optionally a graph of
temperature and pressure over the last week.

![Status Page](/status_page.png?raw=true "Status Page")

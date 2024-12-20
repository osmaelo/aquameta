Aquameta
========

A "datafied" web stack built in PostgreSQL

Contents
--------
- [Status](#status)
- [Overview](#overview)
- [Core Extensions](#core-extensions)
- [User Interface](#user-interface)
- [Download](#download)
- [Install From Source](#install-from-source)
- [Usage](#usage)
- [History](#history)

Status
------

Aquameta is an experimental project, still in early stages of development.  It
is not suitable for production development and should not be used in an
untrusted or mission-critical environment.


Overview
--------

Aquameta is an "all database" web development stack, an attempt to make web
development more modular, simple, coherent and fun by making everything data.
See [Motivation](#motivation) for more.

Under the hood, Aquameta is organized into seven PostgreSQL extensions, that
each corresponds to a layer or tool in a typical web stack.  The database
schema contains ~60 tables, ~50 views and ~90 stored procedures that together
make a minimalist, fairly unopinionated web stack that should be familiar to
most web developers, except that it's all in the database.  A thin
[Golang](http://golang.org/) daemon handles the connection to the database and 
runs a web server. 

Core Extensions
---------------

- [meta](https://github.com/aquameta/meta) - Writable system catalog for
  PostgreSQL, making most database admin tasks (e.g. `CREATE TABLE ...`)
  possible by changing live data.  Makes the database self-aware, and makes it
  possible to represent schema and procedures as data.
- [bundle](extensions/bundle) - Version control system similar to `git` but for
  database rows instead of files.
- [event](extensions/event) - Hooks for monitoring changes to tables, rows and
  columns for inserts, updates and deletes using triggers and fire off events
  via PostgreSQL `NOTIFY`.
- [filesystem](extensions/filesystem) - Makes the file system accessible from
  SQL.
- [endpoint](extensions/endpoint) - Minimalist web request handlers,
  implemented as PostgreSQL procedures:  A REST API, static recources, function
  maps, WebSocket events and URL-pattern templates.
- [widget](extensions/widget) - Web component framework for building modular
  user interface components.  Each widget is a row in the database with columns
  for HTML, CSS and Javascript, and a mechanism for attaching JS dependencies.
- [semantics](extensions/semantics) - Schema decorators, for describing tables
  and columns, and binding custom UI widgets handlers for display and edit.

Together, these extensions make a simple, fairly un-opinionated web stack
(other than that whole all-data thing).


User Interface
--------------

On top of the core extensions, Aquameta has a web-based IDE.  Check out the
demos and such on
[youtube](https://www.youtube.com/channel/UCq0MVZeXqJhcpdDpQQtOs8w).


Motivation
----------

The web stack is very complicated, and frankly a bit of a mess.  Aquameta's
philosophy is that the cause of this mess is the underlying information model
of "files plus syntax".  Under the hood, web stacks *have structure*, but that
structure is latent and heterogeneous.  The heirarchical file system isn't
adequate for handling the level of complexity in the web stack.

Putting things in the database makes them uniform and clean. There are many
architectural advantages, but to highlight a few:

- An all-data web stack means that the various layers of the stack have a
  *shared information model*.  As such, you can combine various layers of the
  stack into a single bundle with ease, because it's all just data.  Whether a
  bundle be an entire app, a Javascript dependency, a collection of user data,
  some database schema and functions, or any other way slice and dice a
  project, as long as it is all data, it makes a single coherent bundle.
- When all the layers are data, you can make tools that work with data,
  generally, and they can apply to all the layers of the stack at the same
  time.

The result is a vast increase in potential for modularity -- reusable
components.  That means we can share code and data in a much more effective
way, and build on each other's work more coherently than in the file-based
paradigm.


Download
--------

Coming soon?


Install From Source
-------------------

1. Install [PostgreSQL](https://www.postgresql.org/download/) version 13 or
   higher.  Once it's installed, make sure the `pg_config` command is in your
   path.  Then create an empty database that Aquameta will be installed into,
   and then create yourself a superuser, typically the same name as your unix
   username:

```bash
# make sure pg_config is present
pg_config --version

# sudo to the postgres user and create a user
sudo -iu postgres
createdb aquameta
psql aquameta
aquameta=# CREATE ROLE eric; -- use your unix username here instead of 'eric'
aquameta=# ALTER ROLE eric superuser login password 'changeme';
```

2. Clone the bleeding-edge source and submodules via git (or download the
latest [source release](https://github.com/aquametalabs/aquameta/releases)):

```bash
git clone --recurse-submodules https://github.com/aquametalabs/aquameta.git
cd aquameta/
```

3. Install Aquameta's extensions into PostgreSQL's `extensions/` directory.  If
you get an error like `make: command not found`, install make via the
`build-essential` package, e.g. `sudo apt install build-essential`.

```bash
cd scripts/
sudo ./make_install_extensions.sh
cd ../
```

*Note for Mac users: this may fail with a cryptic permissions error if
your terminal program does not have Full Disk Access. This can be set
via System Preferences.*

4. Install [Golang](https://golang.org/) version 1.18 or greater, then build
the `./aquameta` binary from aquameta's root directory:

```bash
go --version
go build
```

This should create a binary called `./aquameta`.

5. Edit `conf/boot.conf` to match your PostgreSQL settings.

```bash
cd conf/
cp boot.toml.dist boot.toml
vi boot.toml
cd ../
```

6. Start the Aquameta server:

```bash
./aquameta --help
./aquameta -c conf/boot.toml
```

When Aquameta starts, it checks to see if the core extensions are installed on
the database, and if they are not, it will automatically install them.  Then it
starts the webserver and provides a URL where you can start using the IDE.

Congrats!  The end.

Usage
-----

See the (paltry) [documentation](docs/).


History
-------
Aquameta is the life-work of Eric Hanson for over two decades.  Prototypes have been built in MySQL/PHP, eXist/XML/XQuery/XForms, RDF/SPARQL and more, but finding PostgreSQL was a game changer and this codebase is the first to show some light at the end of the tunnel.

This codebase originated as the startup idea of Aquameta, LLC, a Portland Oregon based software company.  Much blood, sweat and tears was put into formulating and evolving the concept in the RDBMS world.  Huge strides were made by Eric, Mike, Mickey and others in the late 2010s.  But, with zero users, a marginally functional prototype, and a wild idea about how to radically change the very foundations of how we program, the company was unsurprisingly unable to attract institutional investors.  We ran out of money and the company is now inactive.  In retrospect, rebuilding the entire web stack perhaps wasn't the most realistic idea for a startup.

However, work continues on the project as open source, as time, resources and love permits.

Here are some older materials from the early days.  They are woefully out of date from a technical perspective, but conceptually still fairly sound.

* Old blog entries
  * [Introducing Aquameta](https://web.archive.org/web/20150901192639/http://blog.aquameta.com/2015/08/28/introducing-aquameta/)
  * [Aquameta Chapter 1: meta - Writable System Catalog for PostgreSQL](https://web.archive.org/web/20160615075450/http://blog.aquameta.com/2015/08/29/intro-meta/)
  * [Aquameta Chapter 2: filesystem - PostgreSQL <==> File System Bridge](https://web.archive.org/web/20160401073006/http://blog.aquameta.com/2016/01/06/intro-chpater2-filesystem/)
  * [Aquameta Chapter 3: event - The Atoms of Change](https://web.archive.org/web/0/https://blog.aquameta.com/2016/03/21/intro-event/)

* [FLOSS Weekly interview](https://twit.tv/shows/floss-weekly/episodes/449) on TWiT (This Week in Tech)
* [Hacker News](https://news.ycombinator.com/item?id=21281042) discussion
* [Version 0.2 IDE demo](https://www.youtube.com/watch?v=ZOpj8lvNJtg)
* [@twitter](https://twitter.com/aquameta)

Currently, the project has come a very long way, but still has known architectural foot-guns and time-bombs.  It works fine for single users, but being well-equipped for a distributed development ecosystem with independent developers and out-of-step dependency management is a much more challenging problem.  Until the issues with operating at scale are even marginally addressed, we are not trying to lure new users into a swamp where they shalt surely perish.

You're welcome to try the project, it actually works pretty well in a single user or small team environment.  But bring your mud boots and machete: The documentation is wrong, if it exists at all, the architecture can and will change without notice or regard for backwards compatibility.  User experience is not a priority at this time, getting the architecture correct for future users is all that matters.

Architecture nerds and system architectures are welcome.  If you love the idea, help us make it a reality!  There are a vast number of [known issues](https://github.com/aquametalabs/aquameta/issues) in the issue tracker.

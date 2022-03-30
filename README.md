# `pqlbench` command line tool

This project holds the code of a command line tool that can be used to benchmark PromQL query performance across multiple workers/clients against a Promscale instance.

To explore the different options provided by the make command:

    make help

In order to run the project from a clean state:

    make setup install

From now on, there are two options for running the tool.

    pqlbench benchmark -filepath=<file_name> -workers=<num_workers> -promscale.url=<url>

---

    make run filepath=<file_name> workers=<num_workers> promscale.url=<url>

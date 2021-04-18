#!/usr/bin/env python3
import requests
import sys
import os
import csv
import logging
from bs4.builder._htmlparser import HTMLParseError
from bs4 import BeautifulSoup

reportEndpoint = 'http://127.0.0.1:8080/api/v1/reportPackageModification'
updatesCSVFilePath = 'updates.csv'
perPage = 250

logging.basicConfig(format='%(asctime)s %(message)s', level=logging.INFO)

try:
    response = requests.get(
        f'https://aur.archlinux.org/packages/?O=0&SeB=nd&K=&outdated=&SB=l&SO=d&do_Search=Go&PP={perPage}')
    soup = BeautifulSoup(response.text, 'html.parser')
    rowColumns = [row.find_all("td")
                  for row in soup.find('tbody').find_all("tr")]

    if not rowColumns:
        print("Error: No packages found.", file=sys.stderr)
        sys.exit(13)  # ERROR_INVALID_DATA

    lastNUpdates = []
    packages = []

    try:
        with open(updatesCSVFilePath, 'r') as updatesCSV:
            lastNUpdates = list(csv.reader(updatesCSV))[-perPage:]
    except IOError:
        pass

    for (_, columns) in enumerate(rowColumns):
        name = columns[0].a.get_text()
        version = columns[1].get_text()

        if not name or not version:
            print('Error: Invalid package found.', file=sys.stderr)
            sys.exit(13)  # ERROR_INVALID_DATA

        package = [name, version]
        if package in lastNUpdates:
            break

        packages.append(package)

    if packages:
        logging.info('%d new modification(s) found, reporting to controller and appending to CSV.' % len(packages))

        response = requests.post(reportEndpoint,
                                 json=[package[0] for package in packages],
                                 headers={'Content-type': 'application/json'})

        if response.status_code < 299:
            with open(updatesCSVFilePath, 'a') as updatesCSV:
                w = csv.writer(updatesCSV)
                for package in reversed(packages):
                    w.writerow(package)
        else:
            logging.error('Report failed (error code %d)' % response.status_code)
            # TODO: How would we handle an error here? Temporary errors can be ignored, permanent errors
            # would lead to no new packages until it moves to the next page (resulting in 250 packages at once)


except HTMLParseError:
    print("Error: Failed to parse HTML.", file=sys.stderr)
    sys.exit(1)
except IOError as e:
    print("Error: %s" % e, file=sys.stderr)
    sys.exit(1)

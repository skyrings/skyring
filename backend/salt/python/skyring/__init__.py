#!/usr/bin/python
#
# Copyright 2015 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging
import logging.config

_formatString = ('%(asctime)s %(levelname)-8s %(filename)s:%(lineno)s '
                 '%(module)s.%(funcName)s] %(message)s')


def InitLog(logName, filename, logToStderr, level):
    logConfig = {'formatters': {'default': {'format': _formatString,
                                            'datefmt': '%Y-%m-%dT%H:%M:%S%z'}},
                 'handlers': {},
                 'loggers': {logName: {'handlers': [],
                                       'propagate': False}},
                 'root': {'handlers': [], 'level': ''},
                 'version': 1}

    logConfig['root']['level'] = 'INFO' if level == 'NOTICE' else level
    if filename:
        logConfig['handlers'].update({'file': {'class': 'logging.FileHandler',
                                               'formatter': 'default',
                                               'filename': (filename)}})
        logConfig['loggers'][logName]['handlers'].append('file')
        logConfig['root']['handlers'].append('file')

    if logToStderr:
        logConfig['handlers'].update({
            'stderr': {'class': 'logging.StreamHandler',
                       'formatter': 'default',
                       'stream': 'ext://sys.stderr'}})
        logConfig['loggers'][logName]['handlers'].append('stderr')
        logConfig['root']['handlers'].append('stderr')

    if not logConfig['handlers']:
        return False

    logging.config.dictConfig(logConfig)
    return True

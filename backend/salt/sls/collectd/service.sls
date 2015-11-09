{% from "collectd/map.jinja" import collectd_settings with context %}

collectd-service:
  service.running:
    - name: {{ collectd_settings.service }}
    - enable: True
    - require:
      - pkg: {{ collectd_settings.pkg }}

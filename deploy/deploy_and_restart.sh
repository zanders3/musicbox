set -e
chmod +x music
sudo mv music /opt/musicbox/music
sudo service music restart
sudo journalctl -u music.service -b
